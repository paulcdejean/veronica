output "contacts_console_url" {
  value       = "https://console.cloud.google.com/compute/metadata?project=${local.workspace.project_id}"
  description = "Edit the callers' phone numbers here, in the voice-contact-* keys (E.164, for example +15125551234)."
}
