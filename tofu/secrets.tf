# The account auth token is readable at rest from the documented Account
# resource, which lets the whole credential path stay declarative: read it
# here, place it in Secret Manager, and the VM injects it at boot. The token
# therefore appears in OpenTofu state; rotating it in Twilio (create and
# promote a secondary token) followed by an apply and a VM reboot rolls it
# everywhere.
data "http" "twilio_account" {
  url = "https://api.twilio.com/2010-04-01/Accounts/${twilio_api_accounts_incoming_phone_numbers.voice.account_sid}.json"

  request_headers = {
    Authorization = "Basic ${base64encode("${var.twilio_rest_username}:${var.twilio_rest_password}")}"
  }

  lifecycle {
    postcondition {
      condition     = self.status_code == 200
      error_message = "Reading the Twilio account failed; check the twilio_rest_* variables."
    }

    # Twilio redacts auth_token to an empty string (still HTTP 200) when the
    # caller is not permitted to read it, which would silently place an empty
    # credential in Secret Manager.
    postcondition {
      condition     = length(try(jsondecode(self.response_body).auth_token, "")) > 0
      error_message = "The Account response has no auth token; the twilio_rest_* credential cannot read it. Use credentials permitted to view the auth token."
    }
  }
}

resource "google_secret_manager_secret" "voice_env" {
  project   = local.workspace.project_id
  secret_id = "openclaw-${tofu.workspace}-voice-env"

  replication {
    auto {}
  }

  depends_on = [google_project_service.enabled["secretmanager.googleapis.com"]]
}

resource "google_secret_manager_secret_version" "voice_env" {
  secret = google_secret_manager_secret.voice_env.id

  secret_data = <<-EOT
    TWILIO_ACCOUNT_SID=${twilio_api_accounts_incoming_phone_numbers.voice.account_sid}
    TWILIO_AUTH_TOKEN=${jsondecode(data.http.twilio_account.response_body).auth_token}
    TWILIO_FROM_NUMBER=${twilio_api_accounts_incoming_phone_numbers.voice.phone_number}
  EOT
}

resource "google_secret_manager_secret_iam_member" "voice_env_reader" {
  project   = local.workspace.project_id
  secret_id = google_secret_manager_secret.voice_env.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.openclaw.email}"
}
