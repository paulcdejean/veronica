# Credentials for reading the account's auth token over REST. A restricted
# API key scoped to account reads is enough; the account SID and auth token
# also work. Distinct from the provider credentials, which stay in the
# TWILIO_* environment variables.
variable "twilio_rest_username" {
  type      = string
  sensitive = true
}

variable "twilio_rest_password" {
  type      = string
  sensitive = true
}
