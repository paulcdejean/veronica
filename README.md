# Veronica, a phone-callable voice agent

Veronica is a voice agent you reach by calling a phone number. Twilio
answers the call and hands it to OpenAI's Realtime API over SIP;
[gpt-realtime](https://developers.openai.com/api/docs/guides/realtime-sip)
does the talking. Two small Go programs on Google Cloud run the show:

- The **webhook function** (Cloud Run function, `02_telephony/src`) plays
  switchboard: it tells Twilio where to ring the call, and when OpenAI
  asks "should I take this?" it checks the caller against the allowlist —
  rejecting strangers, and dispatching known callers to the driver (a
  Pub/Sub message plus scaling the driver's pool from zero). It never
  answers the call itself.
- The **session driver** (Cloud Run worker pool, `01_session_image/src`)
  is what picks up. Its pool idles at zero instances; when the webhook
  scales it to one, the starting instance pulls the dispatched call,
  accepts it with Veronica's persona (ringing stops here), attaches the
  call's WebSocket, speaks the greeting, and holds the socket for the
  whole call — re-checking the allowlist (a removed caller is hung up on
  within a minute) and, soon, handling tool calls. Because the driver owns
  the call from its first moment, nothing will escape the transcript when
  transcript logging lands. When the call ends the driver scales the pool
  back to zero: the container's lifetime is the call's, and nothing runs
  between calls.

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
- `01_session_image` owns what `02_telephony` needs to pre-exist: the
  session driver's container image (an Artifact Registry repository and a
  `terraform_data` provisioner that runs Cloud Build whenever the source
  changes) and the two empty Secret Manager secrets, so their values can
  be in place before the telephony apply.
- `02_telephony` owns everything a call touches: the webhook function, the
  session worker pool (pinned to the digest behind the image's `:latest`),
  the call-handoff Pub/Sub topic, the two OpenAI secrets, IAM, and the
  Twilio number.

Each `src/` is its own Go module with the same shape: a thin entry point
(`cmd/session/main.go` for the job; `function.go` for the function, where
the Functions Framework requires its registration) over `internal/`
packages split by who they talk to — `realtime` and `openai` speak to the
OpenAI platform, `gcp` is the Google Cloud plumbing (metadata, secrets,
allowlist, Pub/Sub, pool scaling), and `session`/`handler` hold each
program's actual flow. The `gcp` plumbing is deliberately duplicated between the two
modules: each build ships only its own `src/`. The pure logic — signature
verification, caller extraction, allowlist parsing, TwiML — has table
tests; `go test ./...` in either module runs them.

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

[SETUP.md](SETUP.md) walks the whole deployment in order — the three
applies interleaved with the console steps (caller numbers, the OpenAI
webhook endpoint and API key, the secret versions) — from `tofu init`
through a working phone call. The short version: apply `00_contacts`, fill
in numbers, apply `01_session_image`, do the OpenAI steps against the
fixed hostname, apply `02_telephony`, call.

## How a call works

1. Twilio answers the number and POSTs to the function's `/twiml` route,
   which replies with `<Dial answerOnBridge timeout="60"><Sip>` pointing at
   `sip:<project>@sip.api.openai.com` — the call rings through to OpenAI
   while the caller still hears ringing.
2. OpenAI POSTs a signed `realtime.call.incoming` webhook to the function's
   `/openai-webhook` route. The function verifies the signature, reads the
   `voice-contact-*` project metadata, and rejects the call unless the
   caller's number matches an entry (presence of a number authorizes the
   caller).
3. A known caller is dispatched: the function publishes `{call_id,
   caller}` to the handoff topic and scales the session pool from zero to
   one. The caller keeps hearing ringing — nothing has answered yet.
4. The driver instance starts (seconds, not the minutes a Cloud Run *job*
   takes to schedule — that latency is why the pool exists), pulls the
   call, and accepts it with the session config — model, voice, and
   Veronica's instructions, all set in `workspace.tf`. The ringing stops:
   this is the pickup. It attaches the call's WebSocket in the same
   breath, waits a beat for the audio path to bridge, and nudges Veronica
   into speaking the greeting first.
5. The driver stays on the line until the call ends and the socket closes,
   then scales the pool back to zero — unless another call was dispatched
   in the meantime, which it picks up next.

Changing a caller's number is a console metadata edit — it takes effect on
the next call (and, for removals, on live calls within a minute). Adding or
removing a *name* means editing `allowed_callers.txt` and applying
`00_contacts`. Changing the persona, voice, or model is an edit to
`02_telephony/tofu/workspace.tf` and an apply there. Changing the session
driver's code means applying `01_session_image` (which rebuilds the image)
and then `02_telephony` (which rolls the pool to the new digest).

## Operations

```bash
# The webhook function's logs (dispatches, rejects, webhook failures):
gcloud logging read 'resource.type="cloud_run_revision" AND resource.labels.service_name="veronica-webhook"' \
  --project untrusted-agent --freshness 1h --order asc --format 'value(text_payload)'

# The session driver's logs (pickup, greeting, session events, hangups):
gcloud logging read 'resource.type="cloud_run_worker_pool" AND resource.labels.worker_pool_name="veronica-session"' \
  --project untrusted-agent --freshness 1h --order asc --format 'value(text_payload)'

# The pool's state (instance count 1 = a call is being driven):
tofu output -raw session_pool_command | bash

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
