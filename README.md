# Veronica, a phone-callable voice agent

Veronica is a voice agent you reach by calling a phone number. Twilio
answers the call and hands it to OpenAI's Realtime API over SIP;
[gpt-realtime](https://developers.openai.com/api/docs/guides/realtime-sip)
does the talking. The only code is a small Cloudflare Worker that plays
receptionist: it tells Twilio where to send the call, and when OpenAI asks
"should I take this?" it checks the caller against the contacts directory
and answers with Veronica's persona. Audio never touches the Worker.

The repo follows the pinned-provider, workspace, and ordered-layer
conventions from `lightning`, in two layers: `00_contacts` owns the caller
directory (a Workers KV namespace with one entry per name in
`allowed_callers.txt`, the phone numbers typed into the KV dashboard so
they never enter the repo), and `tofu` is the main layer with the Worker,
its custom domain, and the Twilio number.

## Prerequisites

- OpenTofu `1.12.3`
- The same `cloudflare` AWS profile used by `lightning`, with access to its
  Cloudflare R2 bucket named `tofu`
- `CLOUDFLARE_API_TOKEN` exported with Workers Scripts, Workers KV Storage,
  and DNS write access to the zone named in `workspace.tf`
  (`veronica-agent.com`)
- `TWILIO_API_KEY` and `TWILIO_API_SECRET` exported for the provider (the
  account SID and auth token)
- An OpenAI account with API billing enabled (the Realtime API is not
  covered by a ChatGPT subscription); put its project id in `workspace.tf`
  as `openai_project_id`

## Deploy

The contacts layer comes first (in each layer, if the `veronica` workspace
already exists, select it with `tofu workspace select veronica` instead):

```bash
cd 00_contacts
tofu init
tofu workspace new veronica
tofu apply
```

Add each allowed caller's name to `allowed_callers.txt` at the repo root
(one name per line); the apply creates one empty entry per name in the
contacts KV namespace. Then open the printed `contacts_dashboard_url` and
fill in each caller's phone number (E.164, for example `+15125551234`).

Then the main layer:

```bash
cd ../tofu
tofu init
tofu workspace new veronica
tofu apply
```

After the apply, follow [SETUP.md](SETUP.md): it covers the OpenAI console
steps (webhook endpoint, API key) and uploading the Worker's two secrets
with wrangler, through to a working phone call.

## How a call works

1. Twilio answers the number and POSTs to the Worker's `/twiml` route,
   which replies with `<Dial><Sip>` pointing at
   `sip:<project>@sip.api.openai.com` — the call rings through to OpenAI
   while the caller still hears ringing (`answerOnBridge`).
2. OpenAI POSTs a signed `realtime.call.incoming` webhook to the Worker's
   `/openai-webhook` route. The Worker verifies the signature, reads the
   contacts KV namespace, and rejects the call unless the caller's number
   matches an entry (presence of a number authorizes the caller).
3. On accept, the Worker supplies the session: model, voice, and Veronica's
   instructions, all set in `workspace.tf`. It then attaches the call's
   WebSocket just long enough to make Veronica speak the greeting first,
   and disconnects; the call continues between Twilio and OpenAI directly.

Changing a caller's number is a KV dashboard edit — it takes effect on the
next call, nothing to apply or restart. Adding or removing a *name* means
editing `allowed_callers.txt` and applying `00_contacts`. Changing the
persona, voice, or model is an edit to `workspace.tf` and a main-layer
apply.

## Operations

```bash
# Stream the Worker's logs (accepts, rejects, webhook failures):
npx wrangler tail veronica-voice

# Inspect the resolved outputs:
tofu output
```

Call records (including SIP responses from OpenAI) are in the Twilio
console's call log; session activity is on platform.openai.com under Logs.

## Security notes

- The two real credentials — the OpenAI API key and webhook signing secret
  — live only as Worker secrets, uploaded with wrangler; they are never in
  OpenTofu variables, metadata, or state. The main layer's
  `keep_bindings = ["secret_text"]` keeps re-applies from dropping them.
- The webhook route only acts on requests carrying a valid Standard
  Webhooks HMAC signature from OpenAI; the TwiML route serves a static
  instruction whose only payload is the (non-secret) OpenAI project id.
- The callers' phone numbers live in the contacts KV namespace (readable
  by anything with Workers KV access to the account) — never in the repo,
  which carries only their names.
- The inbound allowlist is caller-ID filtering, not authentication; caller
  ID can be spoofed, so do not treat the phone line as a trusted control
  channel.
- Destroying the Twilio number resource releases the number permanently,
  so it carries `prevent_destroy`.
