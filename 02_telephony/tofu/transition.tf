# Transition-only, delete with the cloudflare provider after the demolition
# apply. Zone settings cannot be deleted through the API, so forget the SSL
# setting from state instead of destroying it; the Worker script and custom
# domain resources were simply removed from config and get real destroys.
removed {
  from = cloudflare_zone_setting.voice_ssl

  lifecycle {
    destroy = false
  }
}
