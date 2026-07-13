# Veronica, a phone-callable voice agent

Veronica is a voice agent you reach by calling a phone number. Twilio
answers the call and hands it to OpenAI's Realtime API over SIP;
[gpt-realtime](https://developers.openai.com/api/docs/guides/realtime-sip)
does the talking. Two small Go programs on Google Cloud run the show:

- The **webhook function** (Cloud Run function, `02_telephony/src`) plays
  receptionist: it tells Twilio where to send the call, and when OpenAI
  asks "should I take this?" it checks the caller against the allowlist
  and answers with Veronica's persona.
- The **session driver** (Cloud Run job, `01_session_image/src`) runs one
  execution per call: it attaches the call's WebSocket, speaks the
  greeting, and holds the socket for the whole call — re-checking the
  allowlist (a removed caller is hung up on within a minute) and, soon,
  handling tool calls. When the call ends the process exits and the
  execution completes: the container's lifetime is the call's, so nothing
  runs between calls.

Audio never touches either program — it flows Twilio↔OpenAI directly.

The webhook's public face is a fixed URL on a domain we own,
`voice.veronica-agent.com`: a proxied Cloudflare record points at a global
external load balancer that fronts the function (Cloud Run routes by
hostname, so a bare CNAME can't do it), with a Google-managed certificate
that provisions via DNS authorization so it renews behind the proxy — the
pattern ported from cleverfi-opentofu. The URLs configured at OpenAI and
Twilio are therefore write-once, whatever happens to the function behind
them. The LB's forwarding rule is the stack's one always-on cost
(~$18/month); everything else idles at zero.

The repo follows the pinned-provider, workspace, and ordered-layer
conventions from `lightning`, each layer a `tofu/` root module beside the
`src/` it deploys:

- `00_contacts` owns the caller directory: one `voice-contact-*` project
  metadata entry per name in `allowed_callers.txt`, the numbers typed into
  the Cloud console so they never enter the repo.
- `01_session_image` owns the session driver's container image: an
  Artifact Registry repository and a `terraform_data` provisioner that
  runs Cloud Build whenever the source changes.
- `02_telephony` owns everything a call touches: the webhook function, the
  session job (pinned to the digest behind the image's `:latest`), the two
  OpenAI secrets, IAM, and the Twilio number.

## Prerequisites

- OpenTofu `1.12.3` and the `gcloud` CLI
- The same `cloudflare` AWS profile used by `lightning`, with access to its
  R2 bucket named `tofu` (the state backend)
- Google application-default credentials with access to the project in
  `workspace.tf` (`gcloud auth login` and
  `gcloud auth application-default login`)
- `CLOUDFLARE_API_TOKEN` exported with DNS write access to the zone named
  in `02_telephony/tofu/workspace.tf` (`veronica-agent.com`)
- `TWILIO_API_KEY` and `TWILIO_API_SECRET` exported for the provider (the
  account SID and auth token)
- An OpenAI account with API billing enabled (the Realtime API is not
  covered by a ChatGPT subscription); put its project id in
  `02_telephony/tofu/workspace.tf` as `openai_project_id`

## Deploy

Layers apply in order, each from its `tofu/` directory (if the `veronica`
workspace already exists, `tofu workspace select veronica` instead of
`new`):

```bash
cd 00_contacts/tofu
tofu init
tofu workspace new veronica
tofu apply
```

Add each allowed caller's name to `allowed_callers.txt` at the repo root
(one name per line); the apply creates one empty `voice-contact-*` metadata
entry per name. Then open the printed `contacts_console_url` and fill in
each caller's phone number (E.164, for example `+15125551234`).

```bash
cd ../../01_session_image/tofu
tofu init
tofu workspace new veronica
tofu apply   # runs Cloud Build; finishes with the image pushed
```

```bash
cd ../../02_telephony/tofu
tofu init
tofu workspace new veronica
tofu apply
```

After the applies, follow [SETUP.md](SETUP.md): it covers the OpenAI
console steps (webhook endpoint, API key) and adding the two secret
versions with gcloud, through to a working phone call.

## How a call works

1. Twilio answers the number and POSTs to the function's `/twiml` route,
   which replies with `<Dial><Sip>` pointing at
   `sip:<project>@sip.api.openai.com` — the call rings through to OpenAI
   while the caller still hears ringing (`answerOnBridge`).
2. OpenAI POSTs a signed `realtime.call.incoming` webhook to the function's
   `/openai-webhook` route. The function verifies the signature, reads the
   `voice-contact-*` project metadata, and rejects the call unless the
   caller's number matches an entry (presence of a number authorizes the
   caller).
3. On accept, the function supplies the session — model, voice, and
   Veronica's instructions, all set in `workspace.tf` — and starts one
   execution of the session-driver job with the call's id.
4. The driver attaches the call's WebSocket, waits a beat for the audio
   path to bridge, and nudges Veronica into speaking the greeting first.
   It then stays on the line until the call ends and the socket closes,
   which ends the execution.

Changing a caller's number is a console metadata edit — it takes effect on
the next call (and, for removals, on live calls within a minute). Adding or
removing a *name* means editing `allowed_callers.txt` and applying
`00_contacts`. Changing the persona, voice, or model is an edit to
`02_telephony/tofu/workspace.tf` and an apply there. Changing the session
driver's code means applying `01_session_image` (which rebuilds the image)
and then `02_telephony` (which rolls the job to the new digest).

## Operations

```bash
# The webhook function's logs (accepts, rejects, webhook failures):
gcloud logging read 'resource.type="cloud_run_revision" AND resource.labels.service_name="veronica-webhook"' \
  --project untrusted-agent --freshness 1h --order asc --format 'value(text_payload)'

# The session driver's logs (greeting, session events, hangups):
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="veronica-session"' \
  --project untrusted-agent --freshness 1h --order asc --format 'value(text_payload)'

# One job execution per call:
tofu output -raw session_executions_command | bash

# Inspect the resolved outputs (from 02_telephony/tofu):
tofu output
```

Call records (including SIP responses from OpenAI) are in the Twilio
console's call log; session activity is on platform.openai.com under Logs.

## Security notes

- The two real credentials — the OpenAI API key and webhook signing secret
  — live only as Secret Manager versions added with gcloud; they are never
  in OpenTofu variables or state. Both programs read them at runtime
  (`latest`), so rotation is a new version plus, for the function, its next
  cold start.
- The webhook route only acts on requests carrying a valid Standard
  Webhooks HMAC signature from OpenAI; the TwiML route serves a static
  instruction whose only payload is the (non-secret) OpenAI project id.
- The callers' phone numbers live in project metadata (readable by
  anything in the project with compute read access) — never in the repo,
  which carries only their names.
- The inbound allowlist is caller-ID filtering, not authentication; caller
  ID can be spoofed, so do not treat the phone line as a trusted control
  channel.
- The function is publicly invokable by necessity (Twilio and OpenAI
  cannot present Google credentials), but its ingress only admits traffic
  arriving through the load balancer — the `run.app` URL answers nothing.
  Each identity in the system gets the smallest role that works, and the
  session driver never sees the webhook secret.
- Destroying the Twilio number resource releases the number permanently,
  so it carries `prevent_destroy`.
