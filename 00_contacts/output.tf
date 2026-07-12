output "contacts_dashboard_url" {
  value       = "https://dash.cloudflare.com/${data.cloudflare_accounts.this.result[0].id}/workers/kv/namespaces/${cloudflare_workers_kv_namespace.contacts.id}"
  description = "Edit the callers' phone numbers here (E.164, for example +15125551234)."
}
