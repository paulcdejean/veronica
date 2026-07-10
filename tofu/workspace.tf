locals {
  workspace = local.workspaces[tofu.workspace]

  # This follows lightning's workspace-driven settings pattern while keeping
  # the entire deployment in one OpenTofu root module.
  workspaces = {
    veronica = {
      project_id            = "untrusted-agent"
      region                = "us-central1"
      zone                  = "us-central1-a"
      subnet_cidr           = "10.42.0.0/24"
      machine_type          = "e2-standard-2"
      boot_disk_gb          = 50
      ubuntu_image          = "projects/ubuntu-os-cloud/global/images/family/ubuntu-2604-lts-amd64"
      node_version          = "24.18.0"
      node_linux_x64_sha256 = "55aa7153f9d88f28d765fcdad5ae6945b5c0f98a36881703817e4c450fa76742"
      openclaw_version      = "2026.6.11"
      codex_version         = "0.144.1"
    }
  }
}
