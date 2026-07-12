terraform {
  required_version = "1.12.3"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "7.39.0"
    }

    archive = {
      source  = "hashicorp/archive"
      version = "2.8.0"
    }

    twilio = {
      source  = "twilio/twilio"
      version = "0.18.46"
    }

    # Transition-only: no cloudflare resources remain in this configuration,
    # but the Worker era's script and custom domain are still in state and
    # need their provider to be destroyed (see transition.tf). Delete this
    # block, the provider below, and transition.tf after the demolition
    # apply.
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "5.22.0"
    }
  }

  backend "s3" {
    profile                     = "cloudflare"
    bucket                      = "tofu"
    workspace_key_prefix        = "veronica"
    key                         = basename(abspath("../${path.module}"))
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

provider "google" {
  project = local.workspace.project_id
  region  = local.workspace.region
}

# Reads TWILIO_API_KEY/TWILIO_API_SECRET from the environment, falling back
# to TWILIO_ACCOUNT_SID/TWILIO_AUTH_TOKEN.
provider "twilio" {}

# Transition-only, see above. Reads CLOUDFLARE_API_TOKEN from the environment.
provider "cloudflare" {}
