# From tofu init to a phone call

This walks the whole deployment in order. It assumes the credentials from
README.md's prerequisites are set up.

## 1. Apply tofu/

```bash
cd tofu
tofu init
tofu workspace select veronica   # `tofu workspace new veronica` the first time
tofu apply
```

Creates the contacts KV namespace (one empty key per name in
`allowed_callers.txt`), sets the zone's SSL mode, builds the driver image
via Cloud Build into Artifact Registry (the apply blocks until the tag is
pullable; it rebuilds only when `app/driver` changes), renders the
workspace's `app/wrangler.jsonc` from `app/wrangler.template.jsonc`
(gitignored — the apply stitches in the namespace id, hostname, image
tag, and persona, so nothing is hand-copied between the roots), and —
first apply only — imports the existing Twilio phone number into this
root's state (the `import` block in `twilio.tf`; releasing a number is
permanent, so it is adopted, never recreated).

## 2. Fill in the caller numbers

Open `tofu output -raw contacts_console_url` and give each contact key the
caller's number in E.164 form (`+15125551234`). A number's presence is
what authorizes the caller; edits take effect on the next call, no apply,
no deploy.

## 3. Register the image-pull credential (once)

Cloudflare pulls the driver image from Artifact Registry as the read-only
service account the apply created. Mint it a key and hand it straight to
wrangler (it lands in Cloudflare's Secrets Store; the file must not
linger):

```bash
cd ../app && npm install
PULL_SA=$(cd ../tofu && tofu output -raw image_pull_service_account)
gcloud iam service-accounts keys create gar-pull-key.json --iam-account "$PULL_SA"
cat gar-pull-key.json | npx wrangler containers registries configure \
  us-central1-docker.pkg.dev --gar-email "$PULL_SA" \
  --secret-name veronica-gar-pull --skip-confirmation
rm gar-pull-key.json
```

Key rotation later is the same dance again.

## 4. Deploy the app

```bash
npx wrangler deploy
```

Deploys the Worker, points the container at the image tag in the rendered
config, and claims `voice.veronica-agent.com`. Needs
`CLOUDFLARE_API_TOKEN` (or `npx wrangler login`) — but no Docker: the
image was built remotely in step 1.

## 5. The OpenAI side

If the project, webhook endpoint, and API key survived from the GCP era,
nothing changes — the webhook URL is the same fixed hostname. Otherwise,
on <https://platform.openai.com>, in the project named by
`openai_project_id` in `tofu/workspace.tf`:

1. Make sure billing is enabled (the Realtime API bills per token).
2. Under **Settings → Project → Webhooks**, create an endpoint subscribed
   to the `realtime.call.incoming` event, with the URL from
   `tofu output -raw openai_webhook_url`:

   ```
   https://voice.veronica-agent.com/openai-webhook
   ```

3. Copy the endpoint's signing secret (`whsec_...`), and under **API
   keys** create a key scoped to the project.

## 6. Upload the secrets

```bash
npx wrangler secret put OPENAI_API_KEY
npx wrangler secret put OPENAI_WEBHOOK_SECRET
```

Each command waits for a paste + enter; the values go straight to the
Worker and are never in the repo or OpenTofu state. Rotation later is the
same command again — it takes effect immediately, no redeploy.

## 7. Call the number

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
