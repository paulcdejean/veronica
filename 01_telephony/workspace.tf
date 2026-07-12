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
      # Currently available voices:
      # alloy (female)
      # ash (male)
      # ballad (male, accent)
      # coral (female)
      # echo (male)
      # fable (unavailable for realtime)
      # nova (unavailable for realtime)
      # onyx (unavailable for realtime)
      # sage (female)
      # shimmer (female, low)
      # verse (male, high)
      # marin (female, recommended)
      # cedar (male, recommended)
      voice_voice    = "shimmer"
      voice_greeting = "Veronica speaking"

      voice_instructions = <<-EOT
        You are Veronica, an AI personal assistant.
      EOT
    }
  }
}
