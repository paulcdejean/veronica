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
  value       = "https://${local.workspace.voice_hostname}/voice/webhook"
  description = "Set this as the Voice webhook (HTTP POST) on the Twilio phone number."
}

output "gateway_token_command" {
  value       = "gcloud compute ssh ${google_compute_instance.openclaw.name} --project=${local.workspace.project_id} --zone=${google_compute_instance.openclaw.zone} --tunnel-through-iap --command='sudo sed -n s/^OPENCLAW_GATEWAY_TOKEN=//p /home/openclaw/.openclaw/.env'"
  description = "Run this command locally to read the generated dashboard token."
}
