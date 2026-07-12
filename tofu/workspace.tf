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
      openai_project_id = ""

      voice_model    = "gpt-realtime-2.1-mini"
      voice_voice    = "marin"
      voice_greeting = "Hi, this is Veronica. What can I do for you?"

      voice_instructions = <<-EOT
        You are Veronica, a personal assistant, speaking on a phone call.
        Talk the way a capable person talks on the phone: natural, warm, and
        brief — a sentence or two unless the caller asks for more. Never use
        markdown, lists, or emoji; you are a voice, not a screen. If you do
        not know something, say so plainly instead of guessing.
      EOT
    }
  }
}
