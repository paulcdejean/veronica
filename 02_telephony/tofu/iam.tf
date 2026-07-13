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

# Both ends of the pool's lifecycle scale it — the webhook up on dispatch,
# the driver down when the line goes quiet. developer is the smallest
# predefined role carrying workerpools.update, scoped to just this pool.
resource "google_cloud_run_v2_worker_pool_iam_member" "webhook_scales_session" {
  location = local.workspace.region
  name     = google_cloud_run_v2_worker_pool.session.name
  role     = "roles/run.developer"
  member   = "serviceAccount:${google_service_account.webhook.email}"
}

resource "google_cloud_run_v2_worker_pool_iam_member" "session_scales_itself" {
  location = local.workspace.region
  name     = google_cloud_run_v2_worker_pool.session.name
  role     = "roles/run.developer"
  member   = "serviceAccount:${google_service_account.session.email}"
}

# The handoff: the webhook writes calls, the driver takes them.
resource "google_pubsub_topic_iam_member" "webhook_publishes_calls" {
  topic  = google_pubsub_topic.calls.name
  role   = "roles/pubsub.publisher"
  member = "serviceAccount:${google_service_account.webhook.email}"
}

resource "google_pubsub_subscription_iam_member" "session_pulls_calls" {
  subscription = google_pubsub_subscription.calls.name
  role         = "roles/pubsub.subscriber"
  member       = "serviceAccount:${google_service_account.session.email}"
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
