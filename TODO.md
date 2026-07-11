# TODO

- Backup/restore story for the VM. The instance is treated as disposable
  (any startup-script change recreates it), which currently wipes
  `/home/openclaw`: the OpenAI/Codex device-code logins, the gateway token,
  the caller allowlist in `.env`, and the agent workspace. Figure out how to
  back these up and restore them on a fresh boot so recreation is fully
  hands-off.
