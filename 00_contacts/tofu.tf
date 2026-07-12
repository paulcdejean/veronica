terraform {
  required_version = "1.12.3"

  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "5.22.0"
    }

    # Transition only: destroying the voice-contact-* project metadata
    # entries still in the state requires the provider. Delete this block
    # (and the provider below) after the first apply on veronica-only
    # succeeds.
    google = {
      source  = "hashicorp/google"
      version = "7.39.0"
    }
  }

  backend "s3" {
    profile                     = "cloudflare"
    bucket                      = "tofu"
    workspace_key_prefix        = "veronica"
    key                         = basename(abspath(path.module))
    use_lockfile                = true
    region                      = "auto"
    skip_credentials_validation = true
    skip_metadata_api_check     = true
    skip_region_validation      = true
    skip_requesting_account_id  = true
    skip_s3_checksum            = true
    use_path_style              = true
  }
}

# Reads CLOUDFLARE_API_TOKEN from the environment.
provider "cloudflare" {}

# Transition only — see the note in required_providers.
provider "google" {
  project = "untrusted-agent"
}
