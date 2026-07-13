# DNS authorization lets the Google-managed cert provision and renew while
# the hostname stays behind Cloudflare's proxy (a classic managed cert would
# require DNS to resolve directly to the load balancer). Pattern ported from
# cleverfi-opentofu.
resource "google_certificate_manager_dns_authorization" "voice" {
  name   = "voice-${tofu.workspace}"
  domain = local.workspace.voice_hostname

  depends_on = [google_project_service.services]
}

# Must stay unproxied — Certificate Manager validates against this record.
resource "cloudflare_dns_record" "acme_challenge" {
  zone_id = data.cloudflare_zone.voice.id
  name    = trimsuffix(google_certificate_manager_dns_authorization.voice.dns_resource_record[0].name, ".")
  type    = google_certificate_manager_dns_authorization.voice.dns_resource_record[0].type
  content = trimsuffix(google_certificate_manager_dns_authorization.voice.dns_resource_record[0].data, ".")
  ttl     = 60
  proxied = false

  provisioner "local-exec" {
    command = "sleep 60"
  }
}

resource "google_certificate_manager_certificate" "voice" {
  name = "voice-${tofu.workspace}"

  managed {
    domains            = [local.workspace.voice_hostname]
    dns_authorizations = [google_certificate_manager_dns_authorization.voice.id]
  }

  depends_on = [cloudflare_dns_record.acme_challenge]
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
