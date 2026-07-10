resource "google_service_account" "openclaw" {
  project      = local.workspace.project_id
  account_id   = "openclaw-${tofu.workspace}"
  display_name = "OpenClaw agent (${tofu.workspace})"
}

resource "google_project_iam_member" "admin_iap" {
  project = local.workspace.project_id
  role    = "roles/iap.tunnelResourceAccessor"
  member  = "user:${data.google_client_openid_userinfo.caller.email}"
}

resource "google_project_iam_member" "admin_os_login" {
  project = local.workspace.project_id
  role    = "roles/compute.osAdminLogin"
  member  = "user:${data.google_client_openid_userinfo.caller.email}"
}

resource "google_service_account_iam_member" "admin_use_agent_identity" {
  service_account_id = google_service_account.openclaw.name
  role               = "roles/iam.serviceAccountUser"
  member             = "user:${data.google_client_openid_userinfo.caller.email}"
}
