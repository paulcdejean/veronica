// Veronica's voice front door, as a Cloudflare Worker.
//
// The call path is: Twilio answers the phone number and fetches /twiml,
// which hands the call to OpenAI over SIP. OpenAI then asks us whether to
// take the call by POSTing a signed realtime.call.incoming webhook to
// /openai-webhook; we check the caller against the contacts KV namespace
// and either accept the call with Veronica's persona (model, voice,
// instructions) or reject it. Audio flows Twilio<->OpenAI directly — this
// Worker never touches media. After accepting we briefly attach the call's
// WebSocket to make Veronica speak first, then hang the socket up; the call
// continues on OpenAI's side.
//
// Bindings: CONTACTS (KV, owned by 00_contacts), OPENAI_PROJECT_ID,
// VOICE_MODEL, VOICE_VOICE, VOICE_GREETING, VOICE_INSTRUCTIONS (plain text,
// from workspace.tf), OPENAI_API_KEY and OPENAI_WEBHOOK_SECRET (secrets,
// set with wrangler per SETUP.md — never in OpenTofu).

const TWIML_PATH = "/twiml";
const WEBHOOK_PATH = "/openai-webhook";
const E164 = /^\+[1-9][0-9]{1,14}$/;

// The SIP answer and Twilio's answerOnBridge take a beat to connect the
// caller's audio path after accept; a greeting spoken immediately plays
// into the void and reaches the caller clipped ("...ronica speaking").
const GREETING_SETTLE_MS = 500;

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);
    if (request.method === "POST" && url.pathname === TWIML_PATH) {
      return twiml(env);
    }
    if (request.method === "POST" && url.pathname === WEBHOOK_PATH) {
      return openaiWebhook(request, env, ctx);
    }
    return new Response("not found", { status: 404 });
  },
};

// answerOnBridge keeps the caller hearing ringing until OpenAI accepts, so
// a rejected caller gets a decline instead of a connect-then-hangup.
function twiml(env) {
  const sip = `sip:${env.OPENAI_PROJECT_ID}@sip.api.openai.com;transport=tls`;
  return new Response(
    '<?xml version="1.0" encoding="UTF-8"?>' +
      '<Response><Dial answerOnBridge="true">' +
      `<Sip>${xmlEscape(sip)}</Sip>` +
      "</Dial></Response>",
    { headers: { "Content-Type": "text/xml" } },
  );
}

async function openaiWebhook(request, env, ctx) {
  const body = await request.text();
  if (!(await verifySignature(request, body, env.OPENAI_WEBHOOK_SECRET))) {
    console.error("webhook rejected: bad or missing signature");
    return new Response("bad signature", { status: 401 });
  }

  const event = JSON.parse(body);
  if (event.type !== "realtime.call.incoming") {
    return new Response("ignored", { status: 200 });
  }

  const callId = event.data?.call_id;
  const from = callerNumber(event.data?.sip_headers ?? []);
  const allowed = await allowedNumbers(env);
  if (!from || !allowed.has(from)) {
    console.error(`call rejected: caller ${from ?? "unknown"} not in allowlist`);
    await callControl(env, callId, "reject", { status_code: 603 });
    return new Response("rejected", { status: 200 });
  }

  console.log(`inbound call accepted from ${from} (${callId})`);
  const accepted = await callControl(env, callId, "accept", {
    type: "realtime",
    model: env.VOICE_MODEL,
    instructions: env.VOICE_INSTRUCTIONS,
    audio: { output: { voice: env.VOICE_VOICE } },
  });
  if (accepted) {
    ctx.waitUntil(speakGreeting(env, callId));
  }
  return new Response("accepted", { status: 200 });
}

// Standard Webhooks: HMAC-SHA256 over "<id>.<timestamp>.<body>" with the
// base64 secret after the whsec_ prefix; the signature header holds
// space-separated "v1,<base64>" candidates.
async function verifySignature(request, body, secret) {
  const id = request.headers.get("webhook-id");
  const timestamp = request.headers.get("webhook-timestamp");
  const signatures = request.headers.get("webhook-signature");
  if (!id || !timestamp || !signatures || !secret) {
    return false;
  }
  if (Math.abs(Date.now() / 1000 - Number(timestamp)) > 300) {
    return false;
  }
  const key = await crypto.subtle.importKey(
    "raw",
    Uint8Array.from(atob(secret.replace(/^whsec_/, "")), (c) => c.charCodeAt(0)),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const mac = new Uint8Array(
    await crypto.subtle.sign("HMAC", key, new TextEncoder().encode(`${id}.${timestamp}.${body}`)),
  );
  return signatures.split(" ").some((candidate) => {
    const [version, signature] = candidate.split(",", 2);
    if (version !== "v1" || !signature) {
      return false;
    }
    let given;
    try {
      given = Uint8Array.from(atob(signature), (c) => c.charCodeAt(0));
    } catch {
      return false;
    }
    return given.length === mac.length && crypto.subtle.timingSafeEqual(given, mac);
  });
}

function callerNumber(sipHeaders) {
  const from = sipHeaders.find((h) => h.name?.toLowerCase() === "from");
  return from?.value?.match(/\+[0-9]+/)?.[0] ?? null;
}

// Presence of an E.164 number in the contacts namespace authorizes the
// caller; empty or malformed values are just skipped.
async function allowedNumbers(env) {
  const { keys } = await env.CONTACTS.list();
  const values = await Promise.all(keys.map((k) => env.CONTACTS.get(k.name)));
  return new Set(values.map((v) => (v ?? "").replace(/\s+/g, "")).filter((v) => E164.test(v)));
}

async function callControl(env, callId, action, payload) {
  const response = await fetch(
    `https://api.openai.com/v1/realtime/calls/${encodeURIComponent(callId)}/${action}`,
    {
      method: "POST",
      headers: {
        Authorization: `Bearer ${env.OPENAI_API_KEY}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
    },
  );
  if (!response.ok) {
    console.error(`${action} for call ${callId} failed: ${response.status} ${await response.text()}`);
  }
  return response.ok;
}

// The model only speaks when a response is created, so nudge the greeting
// out rather than waiting for the caller's "hello?". The socket is a
// control channel, not media: closing it after the greeting leaves the
// call running.
async function speakGreeting(env, callId) {
  const upgrade = await fetch(`https://api.openai.com/v1/realtime?call_id=${encodeURIComponent(callId)}`, {
    headers: {
      Upgrade: "websocket",
      Authorization: `Bearer ${env.OPENAI_API_KEY}`,
    },
  });
  const socket = upgrade.webSocket;
  if (!socket) {
    console.error(`greeting skipped: websocket upgrade failed (${upgrade.status})`);
    return;
  }
  socket.accept();
  await new Promise((resolve) => {
    const timer = setTimeout(resolve, 15000);
    const done = () => {
      clearTimeout(timer);
      resolve();
    };
    socket.addEventListener("message", (event) => {
      let message;
      try {
        message = JSON.parse(event.data);
      } catch {
        return; // Non-JSON frames are none of our business.
      }
      // The server reports the session's effective config on attach; logging
      // it shows exactly which instructions the live call is running on.
      if (message.type === "session.created" || message.type === "session.updated") {
        console.log(
          `${message.type}: model=${message.session?.model} instructions=${JSON.stringify(message.session?.instructions)}`,
        );
      }
      if (message.type === "error") {
        console.error(`realtime error: ${JSON.stringify(message.error)}`);
      }
      if (message.type === "response.done") {
        const response = message.response ?? {};
        console.log(
          `greeting done: status=${response.status} details=${JSON.stringify(response.status_details)} usage=${JSON.stringify(response.usage)}`,
        );
        done();
      }
    });
    socket.addEventListener("close", done);
    socket.addEventListener("error", done);
    // Attaching does not replay session.created, so nudge the server into
    // echoing session.updated (a no-op update) to get the effective
    // instructions into the logs.
    socket.send(JSON.stringify({ type: "session.update", session: { type: "realtime" } }));
    setTimeout(() => {
      try {
        socket.send(
          JSON.stringify({
            type: "response.create",
            response: {
              instructions: `Say exactly this and nothing more: "${env.VOICE_GREETING}"`,
              // Hard cap so the greeting can't grow a monologue; later
              // responses (the caller's turns) are unaffected. Output audio
              // runs ~20 tokens per spoken second, so this is ~5 seconds.
              max_output_tokens: 200,
            },
          }),
        );
      } catch {
        // Socket closed while we waited; the call is already over.
      }
    }, GREETING_SETTLE_MS);
  });
  try {
    socket.close();
  } catch {
    // Already closed.
  }
}

function xmlEscape(value) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}
