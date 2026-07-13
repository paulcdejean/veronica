# Identity of the webhook function: reads the allowlist and both secrets,
# publishes call handoffs, and scales the session pool up. Nothing else.
resource "google_service_account" "webhook" {
  account_id   = "voice-webhook-${tofu.workspace}"
  display_name = "Veronica voice webhook function"
  depends_on   = [google_project_service.services]
}

# Identity of the session driver: pulls call handoffs, reads the allowlist
# (for mid-call eviction) and the OpenAI key, and scales its own pool down.
# It never sees the webhook secret.
resource "google_service_account" "session" {
  account_id   = "voice-session-${tofu.workspace}"
  display_name = "Veronica voice session driver"
  depends_on   = [google_project_service.services]
}
