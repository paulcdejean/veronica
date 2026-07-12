# The caller allowlist lives in project metadata owned by the 00_contacts
# layer: one voice-contact-* key per name in allowed_callers.txt, with the
# phone numbers typed into the Compute Engine metadata page so they never
# appear in the repo. The VM reads those entries straight from the metadata
# server on every boot (see the startup script); this plan-time read exists
# only to validate the directory — the instance's preconditions fail the
# plan if it is empty or malformed — and to surface it through the
# voice_contact_numbers output. The google provider has no data source for
# project metadata, so the read is a single GET of the project resource with
# the provider's own token.

data "google_client_config" "default" {}

data "http" "project_metadata" {
  url = "https://compute.googleapis.com/compute/v1/projects/${local.workspace.project_id}"

  request_headers = {
    Authorization = "Bearer ${data.google_client_config.default.access_token}"
  }

  lifecycle {
    postcondition {
      condition     = self.status_code == 200
      error_message = "Reading the project's common instance metadata failed; check ADC credentials and that the compute API is enabled."
    }
  }
}

locals {
  # The allowed callers' names live in allowed_callers.txt at the repo root
  # (one name per line), the same source of truth 00_contacts creates the
  # metadata entries from — including the kebab-cased key derivation, which
  # must match its exactly.
  contact_names = toset([
    for line in split("\n", file("${path.module}/../allowed_callers.txt")) :
    trimspace(line) if trimspace(line) != ""
  ])

  contact_keys = {
    for name in local.contact_names :
    name => "voice-contact-${lower(replace(name, "/[^A-Za-z0-9]+/", "-"))}"
  }

  project_metadata = {
    for item in try(jsondecode(data.http.project_metadata.response_body).commonInstanceMetadata.items, []) :
    item.key => try(item.value, "")
  }

  # Presence of a number authorizes the caller; names whose entry is still
  # empty, or not created yet because 00_contacts has not been applied, are
  # simply skipped (the instance's precondition catches the all-empty case).
  voice_contacts = {
    for name, key in local.contact_keys :
    name => trimspace(local.project_metadata[key])
    if trimspace(try(local.project_metadata[key], "")) != ""
  }

  invalid_voice_contacts = [
    for name, number in local.voice_contacts :
    name if !can(regex("^\\+[1-9][0-9]{1,14}$", number))
  ]
}
