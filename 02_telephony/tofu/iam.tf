resource "google_secret_manager_secret_iam_member" "webhook_reads_key" {
  secret_id = data.google_secret_manager_secret.openai_api_key.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.webhook.email}"
}

resource "google_secret_manager_secret_iam_member" "webhook_reads_webhook_secret" {
  secret_id = data.google_secret_manager_secret.openai_webhook_secret.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.webhook.email}"
}

resource "google_secret_manager_secret_iam_member" "session_reads_key" {
  secret_id = data.google_secret_manager_secret.openai_api_key.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.session.email}"
}

# Executing a job with per-execution overrides (the call id) requires more
# than run.invoker; developer is the smallest predefined role that carries
# it, scoped to just this job.
resource "google_cloud_run_v2_job_iam_member" "webhook_runs_session" {
  location = local.workspace.region
  name     = google_cloud_run_v2_job.session.name
  role     = "roles/run.developer"
  member   = "serviceAccount:${google_service_account.webhook.email}"
}

# The allowlist lives in the project's common instance metadata, which Cloud
# Run's metadata server does not serve; both identities read it through the
# Compute API, and compute.viewer is the smallest predefined role that can.
resource "google_project_iam_member" "webhook_reads_metadata" {
  project = local.workspace.project_id
  role    = "roles/compute.viewer"
  member  = "serviceAccount:${google_service_account.webhook.email}"
}

resource "google_project_iam_member" "session_reads_metadata" {
  project = local.workspace.project_id
  role    = "roles/compute.viewer"
  member  = "serviceAccount:${google_service_account.session.email}"
}
