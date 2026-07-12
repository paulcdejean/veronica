# Veronica's voice front door is a Worker on the voice hostname: Twilio
# fetches TwiML from it that hands each call to OpenAI over SIP, and OpenAI
# asks it (a signed webhook) whether to accept the caller. Audio never
# touches Cloudflare — the Worker is pure control plane.

data "cloudflare_accounts" "this" {
  max_items = 1
}

data "cloudflare_zone" "voice" {
  filter = {
    name = local.workspace.voice_zone
  }
}

# The contacts KV namespace is owned by 00_contacts; find it by title, the
# lightning way of referencing across layers.
data "cloudflare_workers_kv_namespaces" "all" {
  account_id = data.cloudflare_accounts.this.result[0].id

  lifecycle {
    postcondition {
      condition     = contains([for ns in self.result : ns.title], "${tofu.workspace}-contacts")
      error_message = "The ${tofu.workspace}-contacts KV namespace does not exist yet; apply 00_contacts first."
    }
  }
}

locals {
  worker_name = "${tofu.workspace}-voice"

  contacts_namespace_id = one([
    for ns in data.cloudflare_workers_kv_namespaces.all.result :
    ns.id if ns.title == "${tofu.workspace}-contacts"
  ])
}

resource "cloudflare_workers_script" "voice" {
  account_id  = data.cloudflare_accounts.this.result[0].id
  script_name = local.worker_name
  content     = file("${path.module}/files/worker.mjs")
  main_module = "worker.mjs"

  compatibility_date = "2026-07-01"

  # The OpenAI API key and webhook secret are uploaded with wrangler (see
  # SETUP.md), never through OpenTofu, so they stay out of variables and
  # state; keep_bindings stops a re-upload from dropping them.
  keep_bindings = ["secret_text"]

  bindings = [
    {
      name         = "CONTACTS"
      type         = "kv_namespace"
      namespace_id = local.contacts_namespace_id
    },
    {
      name = "OPENAI_PROJECT_ID"
      type = "plain_text"
      text = local.workspace.openai_project_id
    },
    {
      name = "VOICE_MODEL"
      type = "plain_text"
      text = local.workspace.voice_model
    },
    {
      name = "VOICE_VOICE"
      type = "plain_text"
      text = local.workspace.voice_voice
    },
    {
      name = "VOICE_GREETING"
      type = "plain_text"
      text = local.workspace.voice_greeting
    },
    {
      name = "VOICE_INSTRUCTIONS"
      type = "plain_text"
      text = local.workspace.voice_instructions
    },
  ]

  observability = {
    enabled = true

    logs = {
      enabled         = true
      invocation_logs = true
    }
  }

  lifecycle {
    precondition {
      condition     = can(regex("^proj_[A-Za-z0-9]+$", local.workspace.openai_project_id))
      error_message = "Set openai_project_id in workspace.tf to the OpenAI project id (proj_...) the calls should be handed to."
    }
  }
}

resource "cloudflare_workers_custom_domain" "voice" {
  account_id = data.cloudflare_accounts.this.result[0].id
  zone_id    = data.cloudflare_zone.voice.id
  hostname   = local.workspace.voice_hostname
  service    = cloudflare_workers_script.voice.id
}

# The GCP origin days set this to "flexible"; with no origin left, put the
# zone back on full. Worker custom domains terminate TLS at the edge and
# ignore this setting either way.
resource "cloudflare_zone_setting" "voice_ssl" {
  zone_id    = data.cloudflare_zone.voice.id
  setting_id = "ssl"
  value      = "full"

  # Zone settings cannot be deleted through the API; on removal, just forget
  # the resource from state instead of attempting a destroy.
  lifecycle {
    destroy = false
  }
}
