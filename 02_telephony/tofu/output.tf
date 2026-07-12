output "twilio_phone_number" {
  value       = twilio_api_accounts_incoming_phone_numbers.voice.phone_number
  description = "The purchased number; give it to the allowed callers."
}

output "openai_webhook_url" {
  value       = "${google_cloudfunctions2_function.webhook.service_config[0].uri}/openai-webhook"
  description = "Configure this as the realtime.call.incoming webhook endpoint on platform.openai.com."
}

output "contacts_console_url" {
  value       = "https://console.cloud.google.com/compute/metadata?project=${local.workspace.project_id}"
  description = "Edit the callers' phone numbers here, in the voice-contact-* keys (E.164, for example +15125551234)."
}

output "session_executions_command" {
  value       = "gcloud run jobs executions list --job ${google_cloud_run_v2_job.session.name} --region ${local.workspace.region} --project ${local.workspace.project_id}"
  description = "The session-driver job runs one execution per call; this lists them."
}
