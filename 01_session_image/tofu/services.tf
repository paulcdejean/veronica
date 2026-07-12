resource "google_project_service" "services" {
  for_each = toset([
    "artifactregistry.googleapis.com",
    "cloudbuild.googleapis.com",
    "iam.googleapis.com",
    "logging.googleapis.com",
    "storage.googleapis.com",
  ])

  service            = each.value
  disable_on_destroy = false

  lifecycle {
    destroy = false
  }
}
