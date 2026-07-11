locals {
  openclaw_startup_script = templatefile("${path.module}/templates/startup.bash.tftpl", {
    node_version           = local.workspace.node_version
    node_linux_x64_sha256  = local.workspace.node_linux_x64_sha256
    openclaw_version       = local.workspace.openclaw_version
    codex_plugin_version   = local.workspace.codex_plugin_version
    codex_version          = local.workspace.codex_version
    ws_version             = local.workspace.ws_version
    voice_hostname         = local.workspace.voice_hostname
    voice_webhook_port     = local.workspace.voice_webhook_port
    voice_greeting         = local.workspace.voice_greeting
    voice_bridge_sha256    = filesha256("${path.module}/files/voice-bridge.mjs")
    bootstrap_bucket       = google_storage_bucket.bootstrap.name
    voice_bridge_object    = google_storage_bucket_object.voice_bridge.name
    project_id             = local.workspace.project_id
    voice_env_secret       = google_secret_manager_secret.voice_env.secret_id
    voice_allowlist_secret = google_secret_manager_secret.voice_allowlist.secret_id
  })
}

# Any change to the startup script (or the values templated into it) rolls
# the VM to a fresh boot instead of relying on an in-place metadata update
# that an already-provisioned box would only pick up on reboot.
resource "terraform_data" "openclaw_startup_script" {
  triggers_replace = local.openclaw_startup_script
}

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

    access_config {
      nat_ip = google_compute_address.openclaw.address
    }
  }

  metadata = {
    enable-oslogin         = "TRUE"
    block-project-ssh-keys = "TRUE"
    startup-script         = local.openclaw_startup_script
  }

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

  lifecycle {
    replace_triggered_by = [
      terraform_data.openclaw_startup_script,
      # The secret is only read at boot, so a rotated payload (new version
      # resource) must roll the box to take effect. The allowlist secret is
      # deliberately absent: adding a caller should cost a reboot, not the
      # device-code logins a replacement wipes.
      google_secret_manager_secret_version.voice_env,
    ]
  }

  depends_on = [
    google_compute_router_nat.openclaw,
    google_project_service.enabled["iap.googleapis.com"],
    # The first boot reads the voice secrets and downloads the voice bridge,
    # so all must exist and be readable.
    google_secret_manager_secret_version.voice_env,
    google_secret_manager_secret_iam_member.voice_env_reader,
    google_secret_manager_secret_version.voice_allowlist,
    google_secret_manager_secret_iam_member.voice_allowlist_reader,
    google_storage_bucket_iam_member.bootstrap_reader,
  ]
}
