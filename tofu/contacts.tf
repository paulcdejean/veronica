# One KV entry per allowed caller, created empty: the phone number is typed
# into the Cloudflare KV dashboard by hand, which is why applies must never
# touch the value again. Removing a name from allowed_callers.txt deletes
# the entry and with it the caller's authorization. KV key names must not
# contain whitespace, so names are kebab-cased.
#
# The voice Worker reads this namespace at call time (and re-reads it
# during calls), so a number change takes effect on the next call — or
# within a minute on a live one — no apply, no redeploy.

# The token sees multiple accounts; list them all and pick the workspace's
# by exact name here, rather than trusting the API's name filter to do the
# choosing.
data "cloudflare_accounts" "all" {
  max_items = 10
  lifecycle {
    postcondition {
      condition     = coalesce(self.result, null) != null
      error_message = "No cloudflare accounts returned?"
    }
  }
}

locals {
  # cloudflare_account_id = one([
  #   for account in data.cloudflare_accounts.all.result :
  #   account.id if account.name == local.workspace.cloudflare_account_name
  # ])
}

resource "cloudflare_workers_kv_namespace" "contacts" {
  account_id = local.workspace.cloudflare_account_id
  title      = "${tofu.workspace}-contacts"
}

resource "cloudflare_workers_kv" "contact" {
  for_each = local.contact_names

  account_id   = local.workspace.cloudflare_account_id
  namespace_id = cloudflare_workers_kv_namespace.contacts.id
  key_name     = lower(replace(each.value, "/[^A-Za-z0-9]+/", "-"))
  value        = ""

  lifecycle {
    ignore_changes = [value]
  }
}
