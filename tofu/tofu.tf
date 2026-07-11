terraform {
  required_version = "1.12.3"

  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "5.22.0"
    }

    google = {
      source  = "hashicorp/google"
      version = "7.39.0"
    }

    http = {
      source  = "hashicorp/http"
      version = "3.6.0"
    }

    twilio = {
      source  = "twilio/twilio"
      version = "0.18.46"
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

provider "google" {
  project = local.workspace.project_id
  region  = local.workspace.region
  zone    = local.workspace.zone
}

# Reads CLOUDFLARE_API_TOKEN from the environment.
provider "cloudflare" {}

# Reads TWILIO_API_KEY/TWILIO_API_SECRET from the environment, falling back
# to TWILIO_ACCOUNT_SID/TWILIO_AUTH_TOKEN.
provider "twilio" {}
