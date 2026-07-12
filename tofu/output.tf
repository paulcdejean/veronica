output "twilio_phone_number" {
  value       = twilio_api_accounts_incoming_phone_numbers.voice.phone_number
  description = "The purchased number; give it to the allowed callers."
}

output "openai_webhook_url" {
  value       = "https://${cloudflare_workers_custom_domain.voice.hostname}/openai-webhook"
  description = "Configure this as the realtime.call.incoming webhook endpoint on platform.openai.com."
}

output "worker_name" {
  value       = cloudflare_workers_script.voice.script_name
  description = "Pass as --name to wrangler when uploading the Worker's secrets."
}

output "contacts_dashboard_url" {
  value       = "https://dash.cloudflare.com/${data.cloudflare_accounts.this.result[0].id}/workers/kv/namespaces/${local.contacts_namespace_id}"
  description = "Edit the callers' phone numbers here (E.164, for example +15125551234)."
}
