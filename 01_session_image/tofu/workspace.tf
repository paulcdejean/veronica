locals {
  workspace = local.workspaces[tofu.workspace]

  workspaces = {
    veronica = {
      project_id = "untrusted-agent"
      region     = "us-central1"
    }
  }
}
