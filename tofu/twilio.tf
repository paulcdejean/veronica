resource "twilio_api_accounts_incoming_phone_numbers" "voice" {
  area_code     = local.workspace.voice_area_code
  friendly_name = "openclaw-${tofu.workspace}"
  voice_method  = "POST"
  voice_url     = local.voice_webhook_url

  # The plugin only greets an inbound caller after a call.answered event,
  # which Twilio delivers via status callback (CallStatus=in-progress), not
  # via the initial voice webhook (CallStatus=ringing). Without this the
  # call record rots in "ringing" and the caller hears 30s of silence.
  status_callback        = "${local.voice_webhook_url}?type=status"
  status_callback_method = "POST"

  # Destroying this resource releases the phone number back to Twilio,
  # which is not reversible.
  lifecycle {
    prevent_destroy = true
  }
}
