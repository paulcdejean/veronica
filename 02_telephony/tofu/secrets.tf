# Containers only: the values are added as versions with gcloud (see
# SETUP.md) and read at runtime, so the OpenAI key and webhook secret never
# appear in OpenTofu variables or state.

resource "google_secret_manager_secret" "openai_api_key" {
  secret_id = "${tofu.workspace}-openai-api-key"

  replication {
    auto {}
  }

  depends_on = [google_project_service.services]
}

resource "google_secret_manager_secret" "openai_webhook_secret" {
  secret_id = "${tofu.workspace}-openai-webhook-secret"

  replication {
    auto {}
  }

  depends_on = [google_project_service.services]
}
