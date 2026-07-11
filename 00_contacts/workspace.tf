locals {
  workspace = local.workspaces[tofu.workspace]

  # The allowed callers' names live in allowed_callers.txt at the repo root
  # (one name per line), the single source of truth for the metadata keys
  # this layer creates and the main layer reads.
  contact_names = toset([
    for line in split("\n", file("${path.module}/../allowed_callers.txt")) :
    trimspace(line) if trimspace(line) != ""
  ])

  workspaces = {
    veronica = {
      project_id = "untrusted-agent"
    }
  }
}
