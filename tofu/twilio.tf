resource "twilio_api_accounts_incoming_phone_numbers" "voice" {
  area_code     = local.workspace.voice_area_code
  friendly_name = "openclaw-${tofu.workspace}"
  voice_method  = "POST"
  voice_url     = local.voice_webhook_url

  # Destroying this resource releases the phone number back to Twilio,
  # which is not reversible.
  lifecycle {
    prevent_destroy = true
  }
}
