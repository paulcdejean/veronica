resource "google_compute_network" "openclaw" {
  name                    = "openclaw-${tofu.workspace}"
  auto_create_subnetworks = false

  depends_on = [google_project_service.enabled["compute.googleapis.com"]]
}

resource "google_compute_subnetwork" "openclaw" {
  name                     = "openclaw-${tofu.workspace}-${local.workspace.region}"
  region                   = local.workspace.region
  network                  = google_compute_network.openclaw.id
  ip_cidr_range            = local.workspace.subnet_cidr
  private_ip_google_access = true
}

resource "google_compute_router" "openclaw" {
  name    = "openclaw-${tofu.workspace}"
  region  = local.workspace.region
  network = google_compute_network.openclaw.id
}

resource "google_compute_router_nat" "openclaw" {
  name                               = "openclaw-${tofu.workspace}"
  router                             = google_compute_router.openclaw.name
  region                             = local.workspace.region
  nat_ip_allocate_option             = "AUTO_ONLY"
  source_subnetwork_ip_ranges_to_nat = "LIST_OF_SUBNETWORKS"

  subnetwork {
    name                    = google_compute_subnetwork.openclaw.id
    source_ip_ranges_to_nat = ["ALL_IP_RANGES"]
  }
}

resource "google_compute_firewall" "iap_ssh" {
  name      = "openclaw-${tofu.workspace}-iap-ssh"
  network   = google_compute_network.openclaw.name
  direction = "INGRESS"
  priority  = 1000

  # Google-owned source range used by Identity-Aware Proxy TCP forwarding.
  source_ranges = ["35.235.240.0/20"]
  target_tags   = ["openclaw-${tofu.workspace}"]

  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
}
