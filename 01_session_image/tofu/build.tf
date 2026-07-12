# Builds run as a per-workspace SA (the compute default SA would work but is
# shared project-wide, and each workspace must own its own IAM members).
resource "google_service_account" "build" {
  account_id   = "voice-build-${tofu.workspace}"
  display_name = "Veronica session image build"
  depends_on   = [google_project_service.services]
}

resource "google_project_iam_member" "build_builds" {
  project = local.workspace.project_id
  role    = "roles/cloudbuild.builds.builder"
  member  = "serviceAccount:${google_service_account.build.email}"
}

# The builder role's Artifact Registry access is project-dependent; grant the
# push on the repository explicitly so the build never races policy quirks.
resource "google_artifact_registry_repository_iam_member" "build_pushes" {
  repository = google_artifact_registry_repository.voice.name
  role       = "roles/artifactregistry.writer"
  member     = "serviceAccount:${google_service_account.build.email}"
}

# A build kicked off seconds after the grants above loses the IAM propagation
# race (observed on untrusted-agent: "Access to bucket ... denied"). Sleeping
# only on grant creation keeps steady-state applies free.
resource "time_sleep" "build_iam_propagation" {
  create_duration = "5s"
  triggers = {
    builder = google_project_iam_member.build_builds.id
    pusher  = google_artifact_registry_repository_iam_member.build_pushes.id
  }
}

locals {
  image = "${local.workspace.region}-docker.pkg.dev/${local.workspace.project_id}/${google_artifact_registry_repository.voice.repository_id}/session"

  # Any change to the session driver's source replaces the build resource
  # below, which re-runs the provisioner.
  source_hash = sha1(join("", [
    for f in sort(fileset("${path.module}/../src", "**")) :
    filesha1("${path.module}/../src/${f}")
  ]))
}

# The image is built by Cloud Build, triggered from the apply itself: gcloud
# builds submit uploads ../src and blocks until the build succeeds, so when
# this apply finishes the :latest tag is pullable. The telephony layer pins
# the digest behind :latest at its own apply time.
resource "terraform_data" "image" {
  triggers_replace = [local.source_hash, local.image]

  provisioner "local-exec" {
    command = <<-EOT
      gcloud builds submit ${path.module}/../src \
        --project ${local.workspace.project_id} \
        --config ${path.module}/../src/cloudbuild.yaml \
        --substitutions _IMAGE=${local.image}:latest \
        --service-account projects/${local.workspace.project_id}/serviceAccounts/${google_service_account.build.email}
    EOT
  }

  depends_on = [time_sleep.build_iam_propagation]
}
