resource "google_compute_instance" "openclaw" {
  name         = "openclaw-${tofu.workspace}"
  zone         = local.workspace.zone
  machine_type = local.workspace.machine_type
  tags         = ["openclaw-${tofu.workspace}"]

  boot_disk {
    auto_delete = true

    initialize_params {
      image = local.workspace.ubuntu_image
      size  = local.workspace.boot_disk_gb
      type  = "pd-balanced"
    }
  }

  network_interface {
    subnetwork = google_compute_subnetwork.openclaw.id
  }

  metadata = {
    enable-oslogin         = "TRUE"
    block-project-ssh-keys = "TRUE"
  }

  metadata_startup_script = templatefile("${path.module}/templates/startup.bash.tftpl", {
    node_version          = local.workspace.node_version
    node_linux_x64_sha256 = local.workspace.node_linux_x64_sha256
    openclaw_version      = local.workspace.openclaw_version
    codex_plugin_version  = local.workspace.codex_plugin_version
    codex_version         = local.workspace.codex_version
  })

  service_account {
    email  = google_service_account.openclaw.email
    scopes = ["https://www.googleapis.com/auth/cloud-platform"]
  }

  shielded_instance_config {
    enable_secure_boot          = true
    enable_vtpm                 = true
    enable_integrity_monitoring = true
  }

  scheduling {
    automatic_restart   = true
    on_host_maintenance = "MIGRATE"
  }

  depends_on = [
    google_compute_router_nat.openclaw,
    google_project_service.enabled["iap.googleapis.com"],
  ]
}
