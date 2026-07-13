# DNS authorization lets the Google-managed cert provision and renew while
# the hostname stays behind Cloudflare's proxy (a classic managed cert would
# require DNS to resolve directly to the load balancer). Pattern ported from
# cleverfi-opentofu.
resource "google_certificate_manager_dns_authorization" "voice" {
  name   = "voice-${tofu.workspace}"
  domain = local.workspace.voice_hostname

  depends_on = [google_project_service.services]
}

locals {
  # One full TTL for the challenge record to propagate before Certificate
  # Manager validates against it; 60 seconds is also Cloudflare's minimum
  # TTL under general circumstances.
  acme_challenge_ttl_seconds = 60
}

# Must stay unproxied — Certificate Manager validates against this record.
resource "cloudflare_dns_record" "acme_challenge" {
  zone_id = data.cloudflare_zone.voice.id
  name    = trimsuffix(google_certificate_manager_dns_authorization.voice.dns_resource_record[0].name, ".")
  type    = google_certificate_manager_dns_authorization.voice.dns_resource_record[0].type
  content = trimsuffix(google_certificate_manager_dns_authorization.voice.dns_resource_record[0].data, ".")
  ttl     = local.acme_challenge_ttl_seconds
  proxied = false
}

resource "time_sleep" "acme_challenge_propagation" {
  create_duration = "${local.acme_challenge_ttl_seconds}s"
  triggers = {
    record = cloudflare_dns_record.acme_challenge.id
  }
}

resource "google_certificate_manager_certificate" "voice" {
  name = "voice-${tofu.workspace}"

  managed {
    domains            = [local.workspace.voice_hostname]
    dns_authorizations = [google_certificate_manager_dns_authorization.voice.id]
  }

  depends_on = [time_sleep.acme_challenge_propagation]
}

# Global external ALBs can't reference Certificate Manager certs directly;
# they have to be attached to the target proxy through a certificate map.
resource "google_certificate_manager_certificate_map" "voice" {
  name = "voice-${tofu.workspace}"

  depends_on = [google_project_service.services]
}

resource "google_certificate_manager_certificate_map_entry" "voice" {
  name         = "voice-${tofu.workspace}"
  map          = google_certificate_manager_certificate_map.voice.name
  certificates = [google_certificate_manager_certificate.voice.id]
  hostname     = local.workspace.voice_hostname
}
