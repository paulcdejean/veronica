# The session driver: a worker pool that idles at zero instances. The
# webhook publishes the call to the handoff topic and scales the pool to
# one; the starting instance pulls the call, picks it up (the accept
# happens in the driver, so it is attached for the call's entire life),
# and scales the pool back to zero when the line goes quiet. Scaling by
# API does not create revisions, so the count is runtime state — ignored
# below — while everything else about the pool stays declarative.
resource "google_cloud_run_v2_worker_pool" "session" {
  name                = "${tofu.workspace}-session"
  location            = local.workspace.region
  deletion_protection = false

  scaling {
    scaling_mode = "MANUAL"
    # At rest; the webhook and the driver own this number at runtime.
    manual_instance_count = 0
  }

  template {
    service_account = google_service_account.session.email

    containers {
      image = data.google_artifact_registry_docker_image.session.self_link

      env {
        name  = "VOICE_GREETING"
        value = local.workspace.voice_greeting
      }
      env {
        name  = "VOICE_MODEL"
        value = local.workspace.voice_model
      }
      env {
        name  = "VOICE_VOICE"
        value = local.workspace.voice_voice
      }
      env {
        name  = "VOICE_INSTRUCTIONS"
        value = trimspace(local.workspace.voice_instructions)
      }

      env {
        name  = "CALL_SUBSCRIPTION"
        value = google_pubsub_subscription.calls.id
      }
      # The pool's own name, for scaling itself down; a resource cannot
      # reference its own attributes, hence the reconstruction.
      env {
        name  = "WORKER_POOL"
        value = "projects/${local.workspace.project_id}/locations/${local.workspace.region}/workerPools/${tofu.workspace}-session"
      }

      # Resolved at instance start, so a first apply works before the
      # secret has a version — instances only start once calls come in.
      env {
        name = "OPENAI_API_KEY"
        value_source {
          secret_key_ref {
            secret  = data.google_secret_manager_secret.openai_api_key.secret_id
            version = "latest"
          }
        }
      }
    }
  }

  lifecycle {
    ignore_changes = [scaling[0].manual_instance_count]
  }

  depends_on = [google_project_service.services]
}
