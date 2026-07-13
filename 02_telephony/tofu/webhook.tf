# Veronica's voice front door: Twilio fetches TwiML from this function that
# hands each call to OpenAI over SIP, and OpenAI asks it (a signed webhook)
# whether to accept the caller. On accept it starts one session-driver job
# execution (see job.tf) and is done — audio never touches it.

resource "google_storage_bucket" "function_source" {
  name                        = "${local.workspace.project_id}-${tofu.workspace}-voice-source"
  location                    = local.workspace.region
  uniform_bucket_level_access = true
  force_destroy               = true
  depends_on                  = [google_project_service.services]
}

data "archive_file" "webhook_source" {
  type        = "zip"
  source_dir  = "${path.module}/../src"
  output_path = "${path.module}/dist/webhook.zip"
}

resource "google_storage_bucket_object" "webhook_source" {
  bucket = google_storage_bucket.function_source.name
  name   = "webhook-${data.archive_file.webhook_source.output_md5}.zip"
  source = data.archive_file.webhook_source.output_path
}

resource "google_cloudfunctions2_function" "webhook" {
  name     = "${tofu.workspace}-webhook"
  location = local.workspace.region

  build_config {
    runtime         = "go126"
    entry_point     = "webhook"
    service_account = data.google_service_account.build.name
    source {
      storage_source {
        bucket = google_storage_bucket.function_source.name
        object = google_storage_bucket_object.webhook_source.name
      }
    }
  }

  service_config {
    service_account_email = google_service_account.webhook.email
    # The Go runtime idles around 15MiB, so the 128Mi floor fits.
    available_memory   = "128Mi"
    max_instance_count = 3
    timeout_seconds    = 30
    # The load balancer is the only door; the run.app URL answers nothing.
    ingress_settings = "ALLOW_INTERNAL_AND_GCLB"

    environment_variables = {
      OPENAI_PROJECT_ID  = local.workspace.openai_project_id
      VOICE_MODEL        = local.workspace.voice_model
      VOICE_VOICE        = local.workspace.voice_voice
      VOICE_INSTRUCTIONS = trimspace(local.workspace.voice_instructions)
      SESSION_JOB        = "projects/${local.workspace.project_id}/locations/${local.workspace.region}/jobs/${google_cloud_run_v2_job.session.name}"

      # Names of the Secret Manager secrets, not their values: the function
      # reads the latest versions at runtime, so the values never pass
      # through OpenTofu and a first deploy works before they are set.
      OPENAI_API_KEY_SECRET        = google_secret_manager_secret.openai_api_key.secret_id
      OPENAI_WEBHOOK_SECRET_SECRET = google_secret_manager_secret.openai_webhook_secret.secret_id
    }
  }

  lifecycle {
    precondition {
      condition     = can(regex("^proj_[A-Za-z0-9]+$", local.workspace.openai_project_id))
      error_message = "Set openai_project_id in workspace.tf to the OpenAI project id (proj_...) the calls should be handed to."
    }
  }
}

# Twilio and OpenAI cannot present Google credentials, so the function is
# publicly invokable; the webhook signature (verified in code) is what
# authenticates OpenAI, and the TwiML endpoint returns nothing sensitive.
resource "google_cloud_run_v2_service_iam_member" "webhook_public" {
  location = local.workspace.region
  name     = google_cloudfunctions2_function.webhook.name
  role     = "roles/run.invoker"
  member   = "allUsers"
}
