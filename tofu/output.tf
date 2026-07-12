output "instance_name" {
  value = google_compute_instance.openclaw.name
}

output "zone" {
  value = google_compute_instance.openclaw.zone
}

output "ssh_command" {
  value = "gcloud compute ssh ${google_compute_instance.openclaw.name} --project=${local.workspace.project_id} --zone=${google_compute_instance.openclaw.zone} --tunnel-through-iap"
}

output "dashboard_tunnel_command" {
  value = "gcloud compute ssh ${google_compute_instance.openclaw.name} --project=${local.workspace.project_id} --zone=${google_compute_instance.openclaw.zone} --tunnel-through-iap -- -N -L 18789:127.0.0.1:18789"
}

output "public_ip" {
  value = google_compute_address.openclaw.address
}

output "voice_webhook_url" {
  value       = local.voice_webhook_url
  description = "The Voice webhook URL configured on the Twilio phone number."
}

output "twilio_phone_number" {
  value       = twilio_api_accounts_incoming_phone_numbers.voice.phone_number
  description = "The purchased number; use it as TWILIO_FROM_NUMBER on the VM and give it to allowed callers."
}

output "voice_contact_numbers" {
  value       = local.voice_contacts
  description = "The resolved caller allowlist: every contact name whose metadata entry holds a number."
}

output "gateway_token_command" {
  value       = "gcloud compute ssh ${google_compute_instance.openclaw.name} --project=${local.workspace.project_id} --zone=${google_compute_instance.openclaw.zone} --tunnel-through-iap --command='sudo sed -n s/^OPENCLAW_GATEWAY_TOKEN=//p /home/openclaw/.openclaw/.env'"
  description = "Run this command locally to read the generated dashboard token."
}
