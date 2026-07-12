locals {
  workspace = local.workspaces[tofu.workspace]

  # This follows lightning's workspace-driven settings pattern while keeping
  # the entire deployment in one OpenTofu root module.
  workspaces = {
    veronica = {
      voice_zone      = "veronica-agent.com"
      voice_hostname  = "voice.veronica-agent.com"
      voice_area_code = "205"

      # From platform.openai.com -> Settings -> General. Not a credential —
      # it only names the SIP destination the calls are handed to.
      openai_project_id = "proj_qnaIpxtc3PddMxEKQUeqry4O"

      voice_model    = "gpt-realtime-2.1-mini"
      voice_voice    = "marin"
      voice_greeting = "Veronica speaking"

      voice_instructions = <<-EOT
        You are Veronica, an AI personal assistant.
      EOT
    }
  }
}
