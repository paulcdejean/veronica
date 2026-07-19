# From tofu init to a phone call

This walks the whole deployment in order. It assumes the credentials from
README.md's prerequisites are set up, and Docker (or a compatible CLI) is
running for the wrangler steps.

## 1. Apply tofu/

```bash
cd tofu
tofu init
tofu workspace select veronica   # `tofu workspace new veronica` the first time
tofu apply
```

Creates the contacts KV namespace (one empty key per name in
`allowed_callers.txt`), sets the zone's SSL mode, and — first apply only —
imports the existing Twilio phone number into this root's state (the
`import` block in `twilio.tf`; releasing a number is permanent, so it is
adopted, never recreated).

## 2. Fill in the caller numbers

Open `tofu output -raw contacts_console_url` and give each contact key the
caller's number in E.164 form (`+15125551234`). A number's presence is
what authorizes the caller; edits take effect on the next call, no apply,
no deploy.

## 3. Point wrangler at the namespace

Paste `tofu output -raw contacts_namespace_id` into `app/wrangler.jsonc`
as the `CONTACTS` binding's `id`. This is a one-time stitch between the
two roots; the namespace id never changes.

## 4. Clear the GCP-era DNS record (transition only)

The zone still has a `voice` record pointing at the demolished GCP load
balancer. Delete it in the Cloudflare dashboard (DNS → records → `voice`)
so the deploy in the next step can claim the hostname as a Workers custom
domain. Skip this once it's gone — the custom domain manages its own DNS
from then on.

## 5. Deploy the app

```bash
cd ../app
npm install
npx wrangler deploy
```

Builds the driver image from `driver/Dockerfile`, pushes it to
Cloudflare's registry, deploys the Worker, and claims
`voice.veronica-agent.com`. Requires Docker running and
`CLOUDFLARE_API_TOKEN` (or `npx wrangler login`).

## 6. The OpenAI side

If the project, webhook endpoint, and API key survived from the GCP era,
nothing changes — the webhook URL is the same fixed hostname. Otherwise,
on <https://platform.openai.com>, in the project named by
`OPENAI_PROJECT_ID` in `app/wrangler.jsonc`:

1. Make sure billing is enabled (the Realtime API bills per token).
2. Under **Settings → Project → Webhooks**, create an endpoint subscribed
   to the `realtime.call.incoming` event, with the URL from
   `tofu output -raw openai_webhook_url`:

   ```
   https://voice.veronica-agent.com/openai-webhook
   ```

3. Copy the endpoint's signing secret (`whsec_...`), and under **API
   keys** create a key scoped to the project.

## 7. Upload the secrets

```bash
npx wrangler secret put OPENAI_API_KEY
npx wrangler secret put OPENAI_WEBHOOK_SECRET
```

Each command waits for a paste + enter; the values go straight to the
Worker and are never in the repo or OpenTofu state. Rotation later is the
same command again — it takes effect immediately, no redeploy.

## 8. Call the number

Call `tofu output -raw twilio_phone_number` from an allowlisted phone. You
hear a few seconds of ringing while the call's container starts — the
driver is what picks up — then Veronica's greeting. Hang up to end the
call; the container stops with it.

If the call does not connect, watch the logs while calling:

```bash
cd app && npm run tail
```

- Nothing at all: Twilio never reached the Worker or OpenAI never sent the
  webhook — the Twilio console's call log shows the TwiML fetch and the
  SIP leg's response code, and the OpenAI webhook page shows delivery
  attempts.
- `bad or missing signature`: the stored `OPENAI_WEBHOOK_SECRET` does not
  match the endpoint's signing secret.
- `not in allowlist`: the KV value is missing, malformed, or not the
  number you are calling from.
- `dispatching call ...` but no pickup: the driver container failed to
  start or the accept failed — the container's own logs are in the
  dashboard under the worker's **Containers** tab, and a non-2xx dispatch
  makes OpenAI redeliver the webhook, so transient failures retry
  themselves.

## Appendix: GCP-era leftovers

The old roots (`00_contacts`, `01_session_image`, `02_telephony`) are gone
from the repo; their remote state keys (`veronica/00_contacts` etc. in the
R2 `tofu` bucket) are orphaned and can be deleted whenever — but never run
a destroy from an old checkout: the `02_telephony` state still references
the phone number this root now owns. On the GCP side, check for a
lingering global static IP (`gcloud compute addresses list --project
untrusted-agent`) — an unused reserved address bills by the hour.
