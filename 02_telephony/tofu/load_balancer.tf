# The load balancer is what gives the webhook a fixed URL on a domain we
# own: Google's shared Cloud Run front end routes by hostname and would 404
# voice.veronica-agent.com, but the LB terminates it (with the managed cert
# from certificate.tf) and forwards to the function's underlying Cloud Run
# service through a serverless NEG. Its forwarding rule is the one always-on
# cost in the stack (~$18/month). Pattern ported from cleverfi-opentofu.

resource "google_compute_global_address" "voice" {
  name = "voice-${tofu.workspace}"

  depends_on = [google_project_service.services]
}

resource "google_compute_region_network_endpoint_group" "webhook" {
  name                  = "voice-${tofu.workspace}"
  region                = local.workspace.region
  network_endpoint_type = "SERVERLESS"

  cloud_run {
    # A gen2 function is a Cloud Run service with the function's name.
    service = google_cloudfunctions2_function.webhook.name
  }
}

resource "google_compute_backend_service" "webhook" {
  name                  = "voice-${tofu.workspace}"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  protocol              = "HTTPS"

  backend {
    group = google_compute_region_network_endpoint_group.webhook.id
  }
}

resource "google_compute_url_map" "voice" {
  name            = "voice-${tofu.workspace}"
  default_service = google_compute_backend_service.webhook.id
}

resource "google_compute_target_https_proxy" "voice" {
  name            = "voice-${tofu.workspace}"
  url_map         = google_compute_url_map.voice.id
  certificate_map = "//certificatemanager.googleapis.com/${google_certificate_manager_certificate_map.voice.id}"

  depends_on = [google_certificate_manager_certificate_map_entry.voice]
}

resource "google_compute_global_forwarding_rule" "https" {
  name                  = "voice-${tofu.workspace}-https"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  ip_protocol           = "TCP"
  port_range            = "443"
  target                = google_compute_target_https_proxy.voice.id
  ip_address            = google_compute_global_address.voice.id
}
