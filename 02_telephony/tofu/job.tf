# The session driver: one job execution per phone call, started by the
# webhook function with the call's identity overridden in. The container
# holds the call's WebSocket and exits when the call ends, so the
# execution's lifetime — and its billing — is the call's.
resource "google_cloud_run_v2_job" "session" {
  name                = "${tofu.workspace}-session"
  location            = local.workspace.region
  deletion_protection = false

  template {
    template {
      service_account = google_service_account.session.email

      # The Realtime session caps at 60 minutes and the driver leaves at 55
      # on its own; this is the belt if it wedges.
      timeout = "3600s"

      # A failed driver must not rerun: it would re-greet a call that is
      # mid-conversation or long over.
      max_retries = 0

      containers {
        image = data.google_artifact_registry_docker_image.session.self_link

        # Placeholders — every execution overrides these with the real call.
        env {
          name  = "CALL_ID"
          value = ""
        }
        env {
          name  = "CALLER"
          value = ""
        }

        env {
          name  = "VOICE_GREETING"
          value = local.workspace.voice_greeting
        }

        # Resolved at execution time, so a first apply works before the
        # secret has a version — executions only happen once calls do.
        env {
          name = "OPENAI_API_KEY"
          value_source {
            secret_key_ref {
              secret  = google_secret_manager_secret.openai_api_key.secret_id
              version = "latest"
            }
          }
        }
      }
    }
  }

  depends_on = [google_project_service.services]
}
