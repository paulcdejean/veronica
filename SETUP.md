# From a successful apply to a phone call

Everything below assumes `tofu apply` in `tofu/` has completed. Commands
marked `local$` run on your machine from `tofu/`; unmarked commands run on
the VM as your SSH user.

## 1. SSH in and wait for the bootstrap

```bash
local$ tofu output -raw ssh_command
```

Run the printed `gcloud compute ssh` command, then wait for the bootstrap to
finish installing Node, OpenClaw, and the voice bridge. The startup script
runs under `google-startup-scripts.service` (the guest agent, not cloud-init,
so `cloud-init status` reports done while the install is still running); this
blocks until every boot job, the bootstrap included, has finished:

```bash
systemctl is-system-running --wait
```

Then confirm the gateway and the voice bridge came up:

```bash
sudo systemctl status openclaw-gateway voice-bridge --no-pager
```

To watch the bootstrap in progress instead, follow its log with
`sudo journalctl -fu google-startup-scripts.service`.

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
`VOICE_ALLOW_FROM=+15125551234`; separate multiple numbers with commas). Only
allowlisted numbers can reach the agent.

## 4. Restart the voice bridge and verify

The bridge reads the allowlist and credentials from that env file at start:

```bash
sudo systemctl restart voice-bridge
sudo systemctl status voice-bridge --no-pager
```

Then confirm the webhook is reachable through Cloudflare:

```bash
curl -s -o /dev/null -w '%{http_code}\n' https://voice.veronica-agent.com/voice/webhook
```

`405` is the expected answer for a bare GET; a timeout or a `52x` Cloudflare
error means the bridge is not listening or the firewall is not admitting
Cloudflare.

## 5. Call the number

Call the Twilio number from the allowlisted phone. Twilio answers and speaks
the greeting immediately, transcribes what you say, and the agent replies by
voice; hang up to end the call. Each caller number keeps one continuous agent
session across calls.

If the call does not connect, check in order:

```bash
sudo journalctl -u voice-bridge -n 100 --no-pager       # webhook/relay errors
sudo journalctl -u openclaw-gateway -n 100 --no-pager   # agent errors
sudo ss -tlnp | grep 3334                               # bridge listening
```

and the call log in the Twilio console, which shows the exact webhook
response Twilio received.
