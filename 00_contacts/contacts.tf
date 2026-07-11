# One project metadata entry per allowed caller, created empty: the phone
# number is filled in by hand on the Compute Engine metadata page, which is
# why applies must never touch the value again. Removing a name from
# allowed_callers.txt deletes the entry and with it the caller's
# authorization. Metadata keys forbid spaces, so names are kebab-cased; the
# main layer derives the same keys from the same file.
#
# Project metadata is readable by every workload in the project with compute
# read access, which is accepted for phone numbers here.
resource "google_compute_project_metadata_item" "contact" {
  for_each = local.contact_names

  key   = "voice-contact-${lower(replace(each.value, "/[^A-Za-z0-9]+/", "-"))}"
  value = ""

  lifecycle {
    ignore_changes = [value]
  }
}
