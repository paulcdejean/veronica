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

## 3. Caller allowlist — nothing to do

Both the Twilio credentials and the caller allowlist are injected into
`/home/openclaw/.openclaw/.env` automatically on every boot — the `TWILIO_*`
lines from Secret Manager, the `VOICE_ALLOW_FROM` line straight from the
`voice-contact-*` project metadata — so leave those lines alone (edits are
overwritten). The allowlist comes from the contacts directory: names in
`allowed_callers.txt` at the repo root, numbers typed into the Compute
Engine metadata page (`tofu output -raw contacts_console_url` in
`00_contacts/`).

To change a caller's number later: update the metadata page and reboot the
VM with `sudo reboot` — the boot re-reads the metadata and restarts the
services. No apply, no VM replacement, no redoing logins. Adding or removing
a *name* additionally means editing `allowed_callers.txt` and applying
`00_contacts` first.

## 4. Verify the webhook

```bash
sudo systemctl status voice-bridge --no-pager
```

Then confirm it is reachable through Cloudflare:

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
