# Boot artifacts too large or too awkward to inline in instance metadata.
# The startup script downloads them with the VM's service-account token and
# verifies them against a checksum templated into the script, so a changed
# artifact still rolls the VM through the startup-script replace trigger.
resource "google_storage_bucket" "bootstrap" {
  project                     = local.workspace.project_id
  name                        = "${local.workspace.project_id}-openclaw-${tofu.workspace}-bootstrap"
  location                    = local.workspace.region
  uniform_bucket_level_access = true
  public_access_prevention    = "enforced"
  force_destroy               = true

  depends_on = [google_project_service.enabled["storage.googleapis.com"]]
}

resource "google_storage_bucket_object" "voice_bridge" {
  bucket       = google_storage_bucket.bootstrap.name
  name         = "voice-bridge.mjs"
  source       = "${path.module}/files/voice-bridge.mjs"
  content_type = "text/javascript"
}

resource "google_storage_bucket_iam_member" "bootstrap_reader" {
  bucket = google_storage_bucket.bootstrap.name
  role   = "roles/storage.objectViewer"
  member = "serviceAccount:${google_service_account.openclaw.email}"
}
