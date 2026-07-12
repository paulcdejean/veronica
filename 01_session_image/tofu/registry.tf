# Holds the session driver's container image; the telephony layer looks the
# image up here by name (the lightning way of referencing across layers).
resource "google_artifact_registry_repository" "voice" {
  repository_id = tofu.workspace
  format        = "DOCKER"
  description   = "Veronica's session driver images, built by ${tofu.workspace}'s 01_session_image layer."
  depends_on    = [google_project_service.services]
}
