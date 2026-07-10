locals {
  google_project_services = toset([
    "compute.googleapis.com",
    "iap.googleapis.com",
  ])
}

resource "google_project_service" "enabled" {
  for_each = local.google_project_services

  project            = local.workspace.project_id
  service            = each.value
  disable_on_destroy = false
}
