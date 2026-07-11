// Bridges Twilio ConversationRelay to the OpenClaw gateway.
//
// Twilio answers the call and runs STT/TTS on its side; this service only
// exchanges text. It serves the signed voice webhook (returning TwiML that
// hands the call to ConversationRelay) and the ConversationRelay WebSocket,
// forwarding each completed caller utterance to the gateway's OpenResponses
// endpoint and streaming the reply tokens back for synthesis.

import { createServer } from "node:http";
import { createHash, createHmac, randomUUID, timingSafeEqual } from "node:crypto";
import { WebSocketServer } from "ws";

function requireEnv(name) {
  const value = process.env[name];
  if (!value) {
    console.error(`missing required environment variable ${name}`);
    // Matches the unit's RestartPreventExitStatus so a misconfigured boot
    // fails visibly instead of restart-looping.
    process.exit(78);
  }
  return value;
}

const PORT = Number(process.env.VOICE_BRIDGE_PORT ?? "3334");
const HOSTNAME = requireEnv("VOICE_HOSTNAME");
const TWILIO_AUTH_TOKEN = requireEnv("TWILIO_AUTH_TOKEN");
const GATEWAY_TOKEN = requireEnv("OPENCLAW_GATEWAY_TOKEN");
const GREETING = process.env.VOICE_GREETING ?? "Hello.";
const GATEWAY_URL = "http://127.0.0.1:18789/v1/responses";
const WEBHOOK_PATH = "/voice/webhook";
const RELAY_PATH = "/voice/relay";
const NONCE_TTL_MS = 5 * 60 * 1000;
const GATEWAY_TIMEOUT_MS = 120 * 1000;

const CALL_INSTRUCTIONS =
  "You are on a live phone call; the caller hears your words through " +
  "text-to-speech. Reply in short, plain, conversational sentences. Never " +
  "use markdown, headings, lists, code, or emoji, and spell out anything " +
  "that does not read naturally aloud.";

function allowedCallers() {
  return (process.env.VOICE_ALLOW_FROM ?? "")
    .split(",")
    .map((entry) => entry.trim())
    .filter(Boolean);
}

function xmlEscape(text) {
  return text
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&apos;");
}

function safeEqual(a, b) {
  const digestA = createHash("sha256").update(a).digest();
  const digestB = createHash("sha256").update(b).digest();
  return timingSafeEqual(digestA, digestB);
}

// Twilio signs the exact URL it requested plus the form parameters sorted by
// name, HMAC-SHA1 keyed with the account auth token.
function validTwilioSignature(req, body) {
  const signature = req.headers["x-twilio-signature"];
  if (typeof signature !== "string") return false;
  const params = new URLSearchParams(body);
  let signed = `https://${HOSTNAME}${req.url}`;
  for (const key of [...params.keys()].sort()) {
    signed += key + params.get(key);
  }
  const expected = createHmac("sha1", TWILIO_AUTH_TOKEN)
    .update(signed, "utf8")
    .digest("base64");
  return safeEqual(signature, expected);
}

// Single-use nonces tie each ConversationRelay WebSocket back to a webhook
// this process just answered; the WebSocket handshake itself is not signed.
const nonces = new Map();

function issueNonce(from) {
  const nonce = randomUUID();
  nonces.set(nonce, { from, expires: Date.now() + NONCE_TTL_MS });
  return nonce;
}

function consumeNonce(nonce) {
  const now = Date.now();
  for (const [key, entry] of nonces) {
    if (entry.expires < now) nonces.delete(key);
  }
  const entry = nonces.get(nonce);
  if (!entry) return null;
  nonces.delete(nonce);
  return entry;
}

function respondXml(res, twiml) {
  res.writeHead(200, { "content-type": "text/xml" });
  res.end(`<?xml version="1.0" encoding="UTF-8"?>\n${twiml}`);
}

function handleWebhook(req, res, body) {
  if (!validTwilioSignature(req, body)) {
    console.error("webhook rejected: bad or missing Twilio signature");
    res.writeHead(403).end();
    return;
  }
  const params = new URLSearchParams(body);
  const from = params.get("From") ?? "";
  const callSid = params.get("CallSid") ?? "unknown";
  if (!allowedCallers().includes(from)) {
    console.error(`webhook rejected: caller ${from} not in allowlist (${callSid})`);
    respondXml(res, "<Response><Reject/></Response>");
    return;
  }
  console.log(`inbound call accepted from ${from} (${callSid})`);
  const nonce = issueNonce(from);
  respondXml(
    res,
    "<Response><Connect>" +
      `<ConversationRelay url="wss://${xmlEscape(HOSTNAME)}${RELAY_PATH}"` +
      ` welcomeGreeting="${xmlEscape(GREETING)}">` +
      `<Parameter name="nonce" value="${xmlEscape(nonce)}"/>` +
      "</ConversationRelay>" +
      "</Connect></Response>",
  );
}

async function* sseEvents(stream) {
  const decoder = new TextDecoder();
  let buffered = "";
  for await (const chunk of stream) {
    buffered += decoder.decode(chunk, { stream: true });
    let boundary;
    while ((boundary = buffered.indexOf("\n\n")) >= 0) {
      const block = buffered.slice(0, boundary);
      buffered = buffered.slice(boundary + 2);
      const event = { event: "", data: "" };
      for (const rawLine of block.split("\n")) {
        const line = rawLine.replace(/\r$/, "");
        if (line.startsWith("event:")) event.event = line.slice(6).trim();
        if (line.startsWith("data:")) {
          event.data += (event.data ? "\n" : "") + line.slice(5).trim();
        }
      }
      if (event.data) yield event;
    }
  }
}

function sendText(ws, token, last) {
  if (ws.readyState === ws.OPEN) {
    ws.send(JSON.stringify({ type: "text", token, last }));
  }
}

async function relayPromptToGateway(ws, state, prompt) {
  state.inflight?.abort();
  const controller = new AbortController();
  state.inflight = controller;
  const timeout = setTimeout(() => controller.abort(), GATEWAY_TIMEOUT_MS);
  try {
    const response = await fetch(GATEWAY_URL, {
      method: "POST",
      signal: controller.signal,
      headers: {
        authorization: `Bearer ${GATEWAY_TOKEN}`,
        "content-type": "application/json",
      },
      body: JSON.stringify({
        model: "openclaw",
        stream: true,
        input: prompt,
        instructions: CALL_INSTRUCTIONS,
        // A stable key per caller keeps one continuous agent session
        // across calls from the same number.
        user: `voice:${state.from}`,
      }),
    });
    if (!response.ok || !response.body) {
      throw new Error(`gateway responded ${response.status}`);
    }
    for await (const event of sseEvents(response.body)) {
      if (event.data === "[DONE]") break;
      let payload;
      try {
        payload = JSON.parse(event.data);
      } catch {
        continue;
      }
      const type = event.event || payload.type;
      if (type === "response.output_text.delta" && typeof payload.delta === "string") {
        if (controller.signal.aborted) return;
        sendText(ws, payload.delta, false);
      } else if (type === "response.failed") {
        throw new Error("gateway reported a failed response");
      }
    }
    if (!controller.signal.aborted) sendText(ws, "", true);
  } catch (error) {
    if (controller.signal.aborted) return;
    console.error(`gateway relay failed for ${state.from}: ${error.message}`);
    sendText(ws, "Sorry, something went wrong on my end. Could you say that again?", true);
  } finally {
    clearTimeout(timeout);
    if (state.inflight === controller) state.inflight = null;
  }
}

const server = createServer((req, res) => {
  const path = (req.url ?? "").split("?")[0];
  if (path !== WEBHOOK_PATH) {
    res.writeHead(404).end();
    return;
  }
  if (req.method !== "POST") {
    res.writeHead(405, { allow: "POST" }).end();
    return;
  }
  if (req.headers.host !== HOSTNAME) {
    res.writeHead(403).end();
    return;
  }
  let body = "";
  req.setEncoding("utf8");
  req.on("data", (chunk) => {
    body += chunk;
    if (body.length > 64 * 1024) req.destroy();
  });
  req.on("end", () => handleWebhook(req, res, body));
});

const wss = new WebSocketServer({ noServer: true });

server.on("upgrade", (req, socket, head) => {
  const path = (req.url ?? "").split("?")[0];
  if (path !== RELAY_PATH || req.headers.host !== HOSTNAME) {
    socket.destroy();
    return;
  }
  wss.handleUpgrade(req, socket, head, (ws) => wss.emit("connection", ws, req));
});

wss.on("connection", (ws) => {
  const state = { authed: false, from: null, pending: "", inflight: null };

  ws.on("message", (raw) => {
    let message;
    try {
      message = JSON.parse(raw.toString());
    } catch {
      return;
    }
    switch (message.type) {
      case "setup": {
        const entry = consumeNonce(message.customParameters?.nonce);
        if (!entry || entry.from !== message.from) {
          console.error(`relay rejected: setup without a valid nonce (${message.callSid})`);
          ws.close(1008, "unauthorized");
          return;
        }
        state.authed = true;
        state.from = entry.from;
        console.log(`relay session started for ${state.from} (${message.callSid})`);
        break;
      }
      case "prompt": {
        if (!state.authed) {
          ws.close(1008, "unauthorized");
          return;
        }
        state.pending += (state.pending ? " " : "") + (message.voicePrompt ?? "");
        if (message.last) {
          const prompt = state.pending.trim();
          state.pending = "";
          if (prompt) {
            console.log(`caller said: ${prompt}`);
            void relayPromptToGateway(ws, state, prompt);
          }
        }
        break;
      }
      case "interrupt":
        // Twilio already stopped playback; drop the in-flight reply so
        // stale tokens are not spoken after the caller's next turn.
        state.inflight?.abort();
        state.inflight = null;
        break;
      case "error":
        console.error(`conversation relay error: ${message.description}`);
        break;
      default:
        break;
    }
  });

  ws.on("close", () => {
    state.inflight?.abort();
    if (state.from) console.log(`relay session ended for ${state.from}`);
  });
});

server.listen(PORT, "0.0.0.0", () => {
  console.log(`voice bridge listening on 0.0.0.0:${PORT} for ${HOSTNAME}`);
});
