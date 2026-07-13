output "twilio_phone_number" {
  value       = twilio_api_accounts_incoming_phone_numbers.voice.phone_number
  description = "The purchased number; give it to the allowed callers."
}

output "openai_webhook_url" {
  value       = "https://${cloudflare_dns_record.voice.name}/openai-webhook"
  description = "Configure this as the realtime.call.incoming webhook endpoint on platform.openai.com; the hostname is fixed, so this is write-once."
}

output "contacts_console_url" {
  value       = "https://console.cloud.google.com/compute/metadata?project=${local.workspace.project_id}"
  description = "Edit the callers' phone numbers here, in the voice-contact-* keys (E.164, for example +15125551234)."
}

output "session_pool_command" {
  value       = "gcloud run worker-pools describe ${google_cloud_run_v2_worker_pool.session.name} --region ${local.workspace.region} --project ${local.workspace.project_id}"
  description = "The session-driver pool; an instance count of 1 means a call is being driven, 0 means the line is quiet."
}
