# The account auth token is readable at rest from the documented Account
# resource, which lets the whole credential path stay declarative: read it
# here, place it in Secret Manager, and the VM injects it at boot. The token
# therefore appears in OpenTofu state; rotating it in Twilio (create and
# promote a secondary token) followed by an apply and a VM reboot rolls it
# everywhere.
data "http" "twilio_account" {
  url = "https://api.twilio.com/2010-04-01/Accounts/${twilio_api_accounts_incoming_phone_numbers.voice.account_sid}.json"

  request_headers = {
    Authorization = "Basic ${base64encode("${var.TWILIO_API_KEY}:${var.TWILIO_API_SECRET}")}"
  }

  lifecycle {
    postcondition {
      condition     = self.status_code == 200
      error_message = "Reading the Twilio account failed; check the TF_VAR_TWILIO_API_KEY/TF_VAR_TWILIO_API_SECRET variables."
    }

    # Twilio redacts auth_token to an empty string (still HTTP 200) unless
    # the request is authenticated with the auth token itself, which would
    # silently place an empty credential in Secret Manager.
    postcondition {
      condition     = length(try(jsondecode(self.response_body).auth_token, "")) > 0
      error_message = "The Account response has no auth token. Set TF_VAR_TWILIO_API_KEY to the account SID and TF_VAR_TWILIO_API_SECRET to the auth token; real API keys get a redacted response."
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

# The caller allowlist resolved from the contacts project metadata
# (contacts.tf) rides its own secret, separate from voice_env, because the
# two change for different reasons: credential rotation replaces the VM,
# while an allowlist change only needs a reboot to re-run the boot-time
# injection.
resource "google_secret_manager_secret" "voice_allowlist" {
  project   = local.workspace.project_id
  secret_id = "openclaw-${tofu.workspace}-voice-allowlist"

  replication {
    auto {}
  }

  depends_on = [google_project_service.enabled["secretmanager.googleapis.com"]]
}

resource "google_secret_manager_secret_version" "voice_allowlist" {
  secret      = google_secret_manager_secret.voice_allowlist.id
  secret_data = "VOICE_ALLOW_FROM=${join(",", distinct(values(local.voice_contacts)))}\n"

  lifecycle {
    precondition {
      condition     = length(local.voice_contacts) > 0
      error_message = "No contact has a phone number yet. Apply 00_contacts, then fill in at least one voice-contact-* value on the Compute Engine metadata page (see its contacts_console_url output)."
    }

    precondition {
      condition     = length(local.invalid_voice_contacts) == 0
      error_message = "Some contacts have values that are not E.164 numbers (+15125551234 style): ${join(", ", local.invalid_voice_contacts)}. Fix them on the Compute Engine metadata page."
    }
  }
}

resource "google_secret_manager_secret_iam_member" "voice_allowlist_reader" {
  project   = local.workspace.project_id
  secret_id = google_secret_manager_secret.voice_allowlist.secret_id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${google_service_account.openclaw.email}"
}
