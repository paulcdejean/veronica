# OpenClaw on Google Cloud

This repo creates a Google Compute Engine VM for an OpenClaw agent reachable
by phone. It follows the pinned-provider, workspace, and ordered-layer
conventions from `lightning`, in two layers: `00_contacts` owns the caller
directory (one `voice-contact-*` project metadata entry per name in
`allowed_callers.txt`, with the phone numbers typed into the Compute Engine
metadata page so they never enter the repo), and `tofu` is the main layer
with everything else.

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
- The standalone `@openai/codex` CLI `0.144.1`
- An always-on systemd gateway service under a dedicated `openclaw` user
- A small voice bridge service (`tofu/files/voice-bridge.mjs`) that connects
  Twilio ConversationRelay to the gateway for phone calls

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

The contacts layer comes first (in each layer, if the `veronica` workspace
already exists, select it with `tofu workspace select veronica` instead):

```bash
cd 00_contacts
tofu init
tofu workspace new veronica
tofu apply
```

Add each allowed caller's name to `allowed_callers.txt` at the repo root
(one name per line — the source of truth for both layers); the apply creates
one empty `voice-contact-*` project metadata entry per name. Then open the
printed `contacts_console_url` output and fill in each caller's phone number
(E.164, for example `+15125551234`) — the main layer's plan fails until at
least one number is filled in.

Then the main layer:

```bash
cd ../tofu
tofu init
tofu workspace new veronica
tofu apply
```

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
the VM; destroying that resource would release the number permanently, so it
carries `prevent_destroy`. On the Cloudflare side it creates a proxied DNS
record for `voice.veronica-agent.com` pointing at the VM, sets the zone's SSL
mode to `flexible`, and adds an origin rule routing that hostname to the
bridge's port. Twilio's webhook IPs are dynamic and unpublished, so the GCP
firewall admits only Cloudflare's ranges and the bridge verifies each
request's `X-Twilio-Signature` instead.

Calls run on [Twilio ConversationRelay](https://www.twilio.com/docs/voice/conversationrelay):
Twilio answers the call, speaks the greeting, and performs all speech-to-text
and text-to-speech on its side, billed per minute to the same Twilio account.
The `voice-bridge` systemd service on the VM answers the voice webhook with
`<Connect><ConversationRelay>` TwiML, receives each transcribed caller turn
over a WebSocket, forwards it to the gateway's OpenResponses endpoint on
loopback (one persistent agent session per caller number), and streams the
reply text back for synthesis, so speech starts before the agent has finished
composing. No speech vendor or API key beyond Twilio is involved; agent
reasoning stays on the ChatGPT login.

Who may call is decided by the contacts directory: every name in
`allowed_callers.txt` whose `voice-contact-*` metadata entry has a phone
number filled in is allowlisted. The main layer resolves the directory at
plan time (inspect it with the `voice_contact_numbers` output), writes the
numbers to the voice-allowlist secret, and the VM injects `VOICE_ALLOW_FROM`
from it on every boot. To change callers: edit the metadata page (and
`allowed_callers.txt` + `tofu apply` in `00_contacts` if the name is new),
apply the main layer to refresh the secret, and reboot the VM — no
replacement, no login loss.

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
sudo systemctl status openclaw-gateway voice-bridge --no-pager
sudo journalctl -u openclaw-gateway -n 200 --no-pager
sudo journalctl -u voice-bridge -n 200 --no-pager
sudo -iu openclaw openclaw doctor
sudo -iu openclaw openclaw models status
```

To change machine sizing, region, package versions, or deletion protection,
edit the `veronica` entry in `workspace.tf`. Any change that alters the
startup script (package versions included) recreates the VM for a clean
first boot, which wipes `/home/openclaw` — redo the logins from
[SETUP.md](SETUP.md) afterwards (the caller allowlist restores itself from
the contacts directory at boot).

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
- The voice bridge port accepts plain HTTP, but only from Cloudflare's
  published IP ranges, and the bridge rejects requests whose host is not the
  voice hostname or whose Twilio signature does not verify. The
  ConversationRelay WebSocket handshake carries no Twilio signature, so the
  bridge only accepts sessions presenting a single-use nonce it minted while
  answering a signed webhook moments earlier. The Cloudflare-to-origin hop is
  unencrypted by design (`flexible` SSL); public traffic from Twilio to
  Cloudflare is TLS.
- The callers' phone numbers live in project metadata (readable by anything
  in the project with compute read access) and, once resolved, in remote
  state and Secret Manager — never in the repo, which carries only their
  names.
- The inbound allowlist is caller-ID filtering, not authentication; caller ID
  can be spoofed, so do not treat the phone line as a trusted control channel.
