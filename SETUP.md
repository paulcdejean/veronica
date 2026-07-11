# From a successful apply to a phone call

Everything below assumes `tofu apply` in `tofu/` has completed. Commands
marked `local$` run on your machine from `tofu/`; unmarked commands run on
the VM as your SSH user.

## 1. SSH in and wait for the bootstrap

```bash
local$ tofu output -raw ssh_command
```

Run the printed `gcloud compute ssh` command, then wait for cloud-init to
finish installing Node, OpenClaw, and the plugins:

```bash
sudo cloud-init status --wait
```

## 2. Connect the OpenAI account

OpenClaw and the standalone Codex CLI keep separate credential stores, so
run both device-code logins as the service user:

```bash
sudo -iu openclaw openclaw models auth login --provider openai --device-code
sudo -iu openclaw codex login --device-auth
```

Confirm both:

```bash
sudo -iu openclaw openclaw models list --provider openai
sudo -iu openclaw codex login status
```

## 3. Set the caller allowlist

The Twilio credentials and from-number are injected into
`/home/openclaw/.openclaw/.env` automatically on every boot from Secret
Manager, so leave the `TWILIO_*` lines alone (edits to them are overwritten).
The one value the apply cannot know is who may call. On the VM, edit
`/home/openclaw/.openclaw/.env` as root and replace the placeholder
`VOICE_ALLOW_FROM` value with the caller's number in E.164 form (for example
`VOICE_ALLOW_FROM=+15125551234`). Only allowlisted numbers can reach the
agent.

## 4. Restart the gateway and verify

```bash
sudo systemctl restart openclaw-gateway
sudo -iu openclaw openclaw voicecall setup
```

Every check in the setup report should pass. Then confirm the webhook is
reachable through Cloudflare from your machine:

```bash
local$ curl -s -o /dev/null -w '%{http_code}\n' https://voice.veronica-agent.com/voice/webhook
```

Any HTTP status (typically `405` for a bare GET) means the path works; a
timeout or a `52x` Cloudflare error means the plugin is not listening or the
firewall is not admitting Cloudflare.

## 5. Call the number

Call the Twilio number from the allowlisted phone. The agent answers,
transcribes what you say, and replies by voice; hang up to end the session.

If the call does not connect, check in order:

```bash
sudo journalctl -u openclaw-gateway -n 100 --no-pager   # plugin/webhook errors
sudo ss -tlnp | grep 3334                               # webhook server listening
sudo -iu openclaw openclaw voicecall setup              # config self-diagnosis
```

and the call log in the Twilio console, which shows the exact webhook
response Twilio received.
