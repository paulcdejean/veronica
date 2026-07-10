# Twilio delivers voice webhooks from a dynamic, unpublished IP pool, so the
# origin cannot allowlist Twilio directly. Instead the webhook hostname is
# proxied through Cloudflare and the GCP firewall admits only Cloudflare's
# published ranges. Cloudflare terminates public TLS; the Cloudflare-to-origin
# hop is plain HTTP ("flexible"), accepted because it rides the
# Cloudflare/Google interconnect and every request is verified against the
# Twilio webhook signature by the voice-call plugin.

data "cloudflare_zone" "voice" {
  filter = {
    name = local.workspace.voice_zone
  }
}

data "cloudflare_ip_ranges" "cloudflare" {}

resource "cloudflare_dns_record" "voice" {
  zone_id = data.cloudflare_zone.voice.id
  name    = local.workspace.voice_hostname
  type    = "A"
  content = google_compute_address.openclaw.address
  proxied = true
  ttl     = 1
}

resource "cloudflare_zone_setting" "voice_ssl" {
  zone_id    = data.cloudflare_zone.voice.id
  setting_id = "ssl"
  value      = "flexible"
}

# Flexible SSL connects to the origin on port 80; this rule reroutes the
# voice hostname to the plugin's webhook listener instead.
resource "cloudflare_ruleset" "voice_origin" {
  zone_id = data.cloudflare_zone.voice.id
  name    = "openclaw-voice-origin"
  kind    = "zone"
  phase   = "http_request_origin"

  rules = [
    {
      ref         = "voice_webhook_port"
      description = "Route the voice webhook hostname to the plugin port"
      expression  = "(http.host eq \"${local.workspace.voice_hostname}\")"
      action      = "route"
      action_parameters = {
        origin = {
          port = local.workspace.voice_webhook_port
        }
      }
    },
  ]
}
