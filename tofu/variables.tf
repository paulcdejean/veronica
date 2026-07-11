# Credentials for reading the account's auth token over REST. Despite the
# API-key naming, set these to the account SID and auth token themselves:
# Twilio redacts auth_token from reads authenticated with any actual API key
# (standard or restricted), so only the token can fetch the token. Use the
# same values for the provider's TWILIO_API_KEY/TWILIO_API_SECRET env vars.
variable "TWILIO_API_KEY" {
  type      = string
  sensitive = true
}

variable "TWILIO_API_SECRET" {
  type      = string
  sensitive = true
}