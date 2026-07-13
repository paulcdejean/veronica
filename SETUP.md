# From tofu init to a phone call

This walks the whole deployment in order. It assumes the credentials from
README.md's prerequisites are set up, and that in each layer's `tofu/`
directory you have run `tofu init` and selected the workspace
(`tofu workspace new veronica` the first time, `tofu workspace select
veronica` after).

## 1. Apply 00_contacts

```bash
cd 00_contacts/tofu
tofu apply
```

Creates one empty `voice-contact-*` project metadata entry per name in
`allowed_callers.txt` (repo root, one name per line).

## 2. Fill in the caller numbers

Open `tofu output -raw contacts_console_url` and give each
`voice-contact-*` entry the caller's number in E.164 form
(`+15125551234`). A number's presence is what authorizes the caller;
edits take effect on the next call, no apply.

## 3. Apply 01_session_image

```bash
cd ../../01_session_image/tofu
tofu apply
```

Builds and pushes the session driver's image via Cloud Build (the apply
blocks until the build succeeds), and creates the two — still empty —
Secret Manager secrets the next steps fill.

## 4. Create the OpenAI webhook endpoint

On <https://platform.openai.com>, in the project named by
`openai_project_id` in `02_telephony/tofu/workspace.tf`:

1. Make sure billing is enabled (the Realtime API bills per token).
2. Under **Settings → Project → Webhooks**, create an endpoint subscribed
   to the `realtime.call.incoming` event, with the URL
   `https://<voice_hostname>/openai-webhook` — for the veronica workspace
   that is:

   ```
   https://voice.veronica-agent.com/openai-webhook
   ```

   The hostname is a fixed front door we own, which is why this works
   before the telephony layer even exists — and why this endpoint never
   needs editing again.
3. Copy the endpoint's signing secret (`whsec_...`) for step 6.

## 5. Create the OpenAI API key

Under **API keys**, create a key scoped to the same project. The webhook
function uses it to accept/reject calls; the session driver uses it to
attach the call's WebSocket and to hang up.

## 6. Add the secret versions

The two values go straight from your terminal to Secret Manager via
gcloud — never through OpenTofu. `read -rs` keeps the pasted value out of
shell history; each command waits silently for a paste + enter:

```bash
read -rs SECRET && printf '%s' "$SECRET" | \
  gcloud secrets versions add veronica-openai-api-key \
    --project untrusted-agent --data-file=-

read -rs SECRET && printf '%s' "$SECRET" | \
  gcloud secrets versions add veronica-openai-webhook-secret \
    --project untrusted-agent --data-file=-
```

Both programs read the `latest` version at runtime, so rotation later is
just another `versions add` (the webhook function picks it up on its next
cold start).

## 7. Apply 02_telephony

```bash
cd ../../02_telephony/tofu
tofu apply
```

Deploys the webhook function and session job, stands up the load balancer
and certificate behind `voice.veronica-agent.com` (the managed cert takes
a few minutes to provision on the first apply), and points the Twilio
number at the front door. When this apply finishes the system is live.

## 8. Call the number

Call `tofu output -raw twilio_phone_number` from an allowlisted phone.
Veronica speaks the greeting a few seconds after pickup (the session
driver's container is starting); hang up to end the call.

If the call does not connect, check in order:

```bash
# Did the webhook arrive, and was the call accepted or rejected?
gcloud logging read 'resource.type="cloud_run_revision" AND resource.labels.service_name="veronica-webhook"' \
  --project untrusted-agent --freshness 1h --order asc --format 'value(text_payload)'

# Did the session driver start, attach, and greet?
gcloud logging read 'resource.type="cloud_run_job" AND resource.labels.job_name="veronica-session"' \
  --project untrusted-agent --freshness 1h --order asc --format 'value(text_payload)'
```

- Nothing from the function: Twilio never reached it or OpenAI never sent
  the webhook — the Twilio console's call log shows the TwiML fetch and
  the SIP leg's response code, and the OpenAI webhook page shows delivery
  attempts.
- `bad or missing signature`: the stored `veronica-openai-webhook-secret`
  does not match the endpoint's signing secret.
- `not in allowlist`: the metadata value is missing, malformed, or not the
  number you are calling from.
- Accepted but no greeting: the second log command shows whether the
  driver's execution started and what the Realtime session reported; the
  call itself still works (speak first and Veronica answers) even with the
  driver down.
