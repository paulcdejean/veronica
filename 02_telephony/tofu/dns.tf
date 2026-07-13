data "cloudflare_zone" "voice" {
  filter = {
    name = local.workspace.voice_zone
  }
}

# The fixed front door: proxied to the load balancer's global IP. The URL
# configured at OpenAI (webhook endpoint) and Twilio (voice_url) never
# changes, whatever happens to the function behind it.
resource "cloudflare_dns_record" "voice" {
  zone_id = data.cloudflare_zone.voice.id
  name    = local.workspace.voice_hostname
  type    = "A"
  content = google_compute_global_address.voice.address
  ttl     = 1
  proxied = true
}

# Cloudflare must speak TLS to the load balancer (the managed cert makes
# "full" safe). Zone settings cannot be deleted through the API; on removal,
# just forget the resource from state instead of attempting a destroy.
resource "cloudflare_zone_setting" "voice_ssl" {
  zone_id    = data.cloudflare_zone.voice.id
  setting_id = "ssl"
  value      = "full"

  lifecycle {
    destroy = false
  }
}
