locals {
  workspace = local.workspaces[tofu.workspace]

  # The allowed callers' names live in allowed_callers.txt at the repo root
  # (one name per line), the single source of truth for the contacts KV
  # keys this root creates and the voice Worker reads.
  contact_names = toset([
    for line in split("\n", file("${path.module}/../allowed_callers.txt")) :
    trimspace(line) if trimspace(line) != ""
  ])

  # This follows lightning's workspace-driven settings pattern. The OpenAI
  # project id and Veronica's persona (model, voice, greeting,
  # instructions) live with the deployable that carries them now:
  # app/wrangler.jsonc.
  workspaces = {
    veronica = {
      voice_zone      = "veronica-agent.com"
      voice_hostname  = "voice.veronica-agent.com"
      voice_area_code = "205"
    }
  }
}
