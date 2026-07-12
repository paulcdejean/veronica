# From a successful apply to a phone call

Everything below assumes `tofu apply` has completed in both layers.
Commands run on your machine; `tofu output` commands run from `tofu/`.

## 1. Create the OpenAI webhook endpoint

On <https://platform.openai.com>, in the project named by
`openai_project_id`:

1. Make sure billing is enabled (the Realtime API bills per token).
2. Under **Settings → Project → Webhooks**, create an endpoint with the URL
   from `tofu output -raw openai_webhook_url`, subscribed to the
   `realtime.call.incoming` event.
3. Copy the endpoint's signing secret (`whsec_...`) — it is uploaded to the
   Worker in step 3.

## 2. Create the API key

Under **API keys**, create a key scoped to the same project. The Worker
uses it to accept/reject calls and to nudge the greeting.

## 3. Upload the Worker's secrets

The two secrets go straight from your clipboard to Cloudflare via wrangler
— never through OpenTofu. `CLOUDFLARE_API_TOKEN` in the environment is
enough for wrangler; each command prompts for the value:

```bash
npx wrangler secret put OPENAI_API_KEY --name "$(tofu output -raw worker_name)"
npx wrangler secret put OPENAI_WEBHOOK_SECRET --name "$(tofu output -raw worker_name)"
```

Re-running `tofu apply` later keeps these secrets in place
(`keep_bindings`), so this step happens once.

## 4. Fill in the caller numbers

If you have not already: open `tofu output -raw contacts_dashboard_url`
and give each contact entry the caller's number in E.164 form
(`+15125551234`). A number's presence is what authorizes the caller, and
edits take effect on the next call.

## 5. Call the number

Call `tofu output -raw twilio_phone_number` from an allowlisted phone.
Veronica speaks the greeting first; hang up to end the call.

If the call does not connect, check in order:

```bash
npx wrangler tail "$(tofu output -raw worker_name)"   # webhook received? accepted or rejected?
```

- Nothing in the tail: Twilio never reached the Worker or OpenAI never sent
  the webhook — the Twilio console's call log shows the TwiML fetch and the
  SIP leg's response code.
- `bad or missing signature`: the uploaded `OPENAI_WEBHOOK_SECRET` does not
  match the endpoint's signing secret.
- `not in allowlist`: the KV value is missing, malformed, or not the number
  you are calling from.
- Accepted but silent: check the session in platform.openai.com's Logs; if
  the greeting nudge failed the model still answers when you speak first.
