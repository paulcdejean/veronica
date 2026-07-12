locals {
  # The allowed callers' names live in allowed_callers.txt at the repo root
  # (one name per line), the single source of truth for the keys this layer
  # creates in the contacts namespace.
  contact_names = toset([
    for line in split("\n", file("${path.module}/../allowed_callers.txt")) :
    trimspace(line) if trimspace(line) != ""
  ])
}
