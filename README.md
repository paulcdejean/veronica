# OpenClaw on Google Cloud

This single OpenTofu root creates a private Google Compute Engine VM for an
OpenClaw agent. It follows the pinned-provider and workspace conventions from
`lightning`, without splitting the deployment into ordered layers.

The VM has no public IP. Cloud NAT provides outbound access, and the only
ingress rule is SSH from Google IAP. OpenClaw listens on loopback with a
randomly generated gateway token. The bootstrap installs:

- Ubuntu 26.04 LTS from the `ubuntu-2604-lts-amd64` image family
- Node.js LTS `24.18.0`, verified against its published SHA-256 checksum
- OpenClaw `2026.6.11`, configured for `openai/gpt-5.5`
- The official `@openclaw/codex` app-server plugin/runtime `2026.6.11`
- The standalone `@openai/codex` CLI `0.144.1`
- An always-on systemd gateway service under a dedicated `openclaw` user

## Prerequisites

- OpenTofu `1.12.3`
- Google Cloud CLI authenticated with permission to enable APIs and create the
  resources in this module
- The same `cloudflare` AWS profile used by `lightning`, with access to its
  Cloudflare R2 bucket named `tofu`
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

Print the SSH command and run it, then wait for cloud-init on the VM before
onboarding:

```bash
tofu output -raw ssh_command
# Run the printed command, then on the VM:
sudo cloud-init status --wait
```

## Connect the OpenAI account

OpenClaw and the standalone Codex CLI keep separate credential stores. Run both
device-code logins as the service user; neither credential is placed in
OpenTofu variables, metadata, or state:

```bash
tofu output -raw ssh_command
# Run the printed command, then on the VM:
sudo -iu openclaw openclaw models auth login --provider openai --device-code
sudo -iu openclaw codex login --device-auth
sudo systemctl restart openclaw-gateway
sudo -iu openclaw openclaw models list --provider openai
sudo -iu openclaw codex login status
```

The first login powers OpenClaw's native Codex app-server runtime through your
ChatGPT/Codex subscription. The second makes the standalone `codex` command
usable for direct terminal work on this VM.

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
edit the `veronica` entry in `workspace.tf`. Package version changes update the
startup-script metadata; restart the VM to rerun the bootstrap on an existing
instance.

## Security notes

- OAuth credentials live only in `/home/openclaw`, not in OpenTofu state.
- The gateway token is generated on the VM and stored at
  `/home/openclaw/.openclaw/.env` with restricted permissions. OpenClaw loads
  this global environment file for both CLI and systemd gateway use.
- The VM service account receives no project IAM roles by default.
- OS Login is enabled and project-wide SSH keys are blocked.
- Anyone with root access to the VM can read the agent credentials and gateway
  token; keep IAP and OS Login grants narrow.
