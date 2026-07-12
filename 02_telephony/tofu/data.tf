# The session driver's image is owned by 01_session_image; look it up by
# name, the lightning way of referencing across layers. If that layer has
# not been applied (no repository, or no successfully built :latest image),
# this read fails and stops the plan — apply 01_session_image first. The
# job pins the digest behind :latest, so a rebuilt image rolls the job on
# the next apply here.
data "google_artifact_registry_docker_image" "session" {
  location      = local.workspace.region
  repository_id = tofu.workspace
  image_name    = "session:latest"
}

# The build service account is also owned by 01_session_image, along with
# its cloudbuild.builds.builder grant; the function's source builds run as
# it. Missing means the same thing: apply 01_session_image first.
data "google_service_account" "build" {
  account_id = "voice-build-${tofu.workspace}"
}
