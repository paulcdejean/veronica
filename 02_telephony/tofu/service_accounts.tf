# Identity of the webhook function: reads the allowlist and both secrets,
# and may start session-driver executions. Nothing else.
resource "google_service_account" "webhook" {
  account_id   = "voice-webhook-${tofu.workspace}"
  display_name = "Veronica voice webhook function"
  depends_on   = [google_project_service.services]
}

# Identity of the session-driver job: reads the allowlist (for mid-call
# eviction) and the OpenAI key. It never sees the webhook secret.
resource "google_service_account" "session" {
  account_id   = "voice-session-${tofu.workspace}"
  display_name = "Veronica voice session driver"
  depends_on   = [google_project_service.services]
}
