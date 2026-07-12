resource "twilio_api_accounts_incoming_phone_numbers" "voice" {
  area_code     = local.workspace.voice_area_code
  friendly_name = "veronica-${tofu.workspace}"
  voice_method  = "POST"
  voice_url     = "${google_cloudfunctions2_function.webhook.service_config[0].uri}/twiml"

  # Destroying this resource releases the phone number back to Twilio,
  # which is not reversible.
  lifecycle {
    prevent_destroy = true
  }
}
