# OpenClaw on Google Cloud

This single OpenTofu root creates a Google Compute Engine VM for an OpenClaw
agent reachable by phone. It follows the pinned-provider and workspace
conventions from `lightning`, without splitting the deployment into ordered
layers.

The VM has a static public IP so the agent can receive Twilio voice webhooks,
but ingress is limited to SSH from Google IAP and the voice webhook port from
Cloudflare's published IP ranges. The webhook hostname is proxied through
Cloudflare (which terminates public TLS from Twilio), and the OpenClaw gateway
itself listens on loopback with a randomly generated token. The bootstrap
installs:

- Ubuntu 26.04 LTS from the `ubuntu-2604-lts-amd64` image family
- Node.js LTS `24.18.0`, verified against its published SHA-256 checksum
- OpenClaw `2026.6.11`, configured for `openai/gpt-5.5`
- The official `@openclaw/codex` app-server plugin/runtime `2026.6.11`
- The official `@openclaw/voice-call` plugin `2026.6.11` for phone calls
- The standalone `@openai/codex` CLI `0.144.1`
- An always-on systemd gateway service under a dedicated `openclaw` user

## Prerequisites

- OpenTofu `1.12.3`
- Google Cloud CLI authenticated with permission to enable APIs and create the
  resources in this module
- The same `cloudflare` AWS profile used by `lightning`, with access to its
  Cloudflare R2 bucket named `tofu`
- `CLOUDFLARE_API_TOKEN` exported with DNS, Zone Settings, and Zone Rulesets
  write access to the zone named in `workspace.tf` (`veronica-agent.com`)
- `TWILIO_API_KEY` and `TWILIO_API_SECRET` exported for the provider, and the
  same values mirrored as `TF_VAR_TWILIO_API_KEY` and `TF_VAR_TWILIO_API_SECRET`
  for reading the auth token. Despite the names, set them to the **account SID
  and auth token**, not a real API key: Twilio's security model offers no
  scoped credential that can read the auth token, and redacts it to an empty
  string on any read not authenticated by the token itself (the plan rejects
  that)
- Billing enabled on the target Google Cloud project

## Deploy

```bash
cd tofu
tofu init
tofu workspace new veronica
tofu apply
```

If `veronica` already exists, select it with `tofu workspace select veronica`.
The project is selected in `workspace.tf`, and the authenticated Google caller
is granted IAP tunneling, OS Login, and service-account use.

## Onboard the agent

After a successful apply, follow [SETUP.md](SETUP.md) step by step: it covers
the OpenAI device-code logins, the Twilio credentials on the VM, and
verification through to a working phone call. No credential is ever placed in
OpenTofu variables, metadata, or state.

## Open the terminal UI

After connecting to the VM over SSH, run the TUI as the `openclaw` service
user so it uses the agent's configuration and credentials:

```bash
sudo -iu openclaw openclaw tui
```

The TUI connects to the local gateway and opens the default `main` session.
Type a message and press Enter, use `/status` to inspect the connection, and
press `Ctrl+C` to exit. Check the gateway if the TUI cannot connect:

```bash
sudo systemctl status openclaw-gateway --no-pager
```

For an embedded local session that bypasses the gateway, run
`sudo -iu openclaw openclaw chat`. Normal use should go through
`openclaw tui`.

## Phone calls (Twilio)

The apply provisions the whole call path. On the Twilio side it purchases a
voice number (area code set in `workspace.tf`) and points its Voice webhook at
the plugin; destroying that resource would release the number permanently, so
it carries `prevent_destroy`. On the Cloudflare side it creates a proxied DNS
record for `voice.veronica-agent.com` pointing at the VM, sets the zone's SSL
mode to `flexible`, and adds an origin rule routing that hostname to the
plugin's webhook port. Twilio's webhook IPs are dynamic and unpublished, so
the GCP firewall admits only Cloudflare's ranges and the plugin verifies each
request's `X-Twilio-Signature` instead.

The remaining on-VM configuration (Twilio credentials, caller allowlist,
verification) is covered in [SETUP.md](SETUP.md).

Inbound calls use turn-based speech (Twilio transcribes, the agent replies via
TTS). For lower-latency full-duplex conversation, enable `realtime` in the
plugin config later; that requires an OpenAI or Gemini API key, which the
ChatGPT device-code login does not provide.

## Open the dashboard

In one local terminal, print and run the IAP-backed SSH tunnel command:

```bash
tofu output -raw dashboard_tunnel_command
```

In another terminal, print and run the token retrieval command:

```bash
tofu output -raw gateway_token_command
```

Then open <http://127.0.0.1:18789/> and enter that token. The dashboard is not
exposed directly to the internet.

## Operations

```bash
# Connect using the printed SSH command.
tofu output -raw ssh_command

# Then run on the VM:
sudo systemctl status openclaw-gateway --no-pager
sudo journalctl -u openclaw-gateway -n 200 --no-pager
sudo -iu openclaw openclaw doctor
sudo -iu openclaw openclaw models status
```

To change machine sizing, region, package versions, or deletion protection,
edit the `veronica` entry in `workspace.tf`. Any change that alters the
startup script (package versions included) recreates the VM for a clean
first boot, which wipes `/home/openclaw` — redo the logins and caller
allowlist from [SETUP.md](SETUP.md) afterwards.

## Security notes

- OAuth credentials live only in `/home/openclaw`, not in OpenTofu state.
- The Twilio auth token is the one credential OpenTofu handles: it is read
  from the Account API at plan time, stored in remote state and in Secret
  Manager, and injected into the VM's env file at boot. Only the VM service
  account can access the secret. To rotate, create and promote a secondary
  auth token in Twilio, then apply; the changed secret version replaces the
  VM so the fresh boot picks it up.
- The gateway token is generated on the VM and stored at
  `/home/openclaw/.openclaw/.env` with restricted permissions. OpenClaw loads
  this global environment file for both CLI and systemd gateway use.
- The VM service account receives no project IAM roles by default.
- OS Login is enabled and project-wide SSH keys are blocked.
- Anyone with root access to the VM can read the agent credentials and gateway
  token; keep IAP and OS Login grants narrow.
- The voice webhook port accepts plain HTTP, but only from Cloudflare's
  published IP ranges, and the plugin rejects requests whose host is not the
  voice hostname or whose Twilio signature does not verify. The
  Cloudflare-to-origin hop is unencrypted by design (`flexible` SSL); public
  traffic from Twilio to Cloudflare is TLS.
- The inbound allowlist is caller-ID filtering, not authentication; caller ID
  can be spoofed, so do not treat the phone line as a trusted control channel.
