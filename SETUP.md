# From a successful apply to a phone call

Everything below assumes `tofu apply` has completed in all three layers
(see README.md). `tofu output` commands run from `02_telephony/tofu/`.

## 1. Create the OpenAI webhook endpoint

On <https://platform.openai.com>, in the project named by
`openai_project_id`:

1. Make sure billing is enabled (the Realtime API bills per token).
2. Under **Settings → Project → Webhooks**, create an endpoint with the URL
   from `tofu output -raw openai_webhook_url`, subscribed to the
   `realtime.call.incoming` event. The hostname is the fixed
   `voice.veronica-agent.com` front door, so this endpoint never needs
   editing again.
3. Copy the endpoint's signing secret (`whsec_...`) — it is stored in
   step 3.

## 2. Create the API key

Under **API keys**, create a key scoped to the same project. The webhook
function uses it to accept/reject calls; the session driver uses it to
attach the call's WebSocket and to hang up.

## 3. Add the secret versions

The two secrets go straight from your terminal to Secret Manager via
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

Both programs read the `latest` version at runtime, so this step happens
once and rotation is just another `versions add` (the webhook function
picks a rotation up on its next cold start).

## 4. Fill in the caller numbers

If you have not already: open `tofu output -raw contacts_console_url` and
give each `voice-contact-*` entry the caller's number in E.164 form
(`+15125551234`). A number's presence is what authorizes the caller, and
edits take effect on the next call.

## 5. Call the number

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
