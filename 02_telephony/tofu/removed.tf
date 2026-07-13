# One-time forget: the two secrets were created here before they moved to
# 01_session_image, which imports them (see its imports.tf). Forgetting —
# not destroying — hands them over without touching the real resources.
# Delete this file after the apply that forgets them.

removed {
  from = google_secret_manager_secret.openai_api_key

  lifecycle {
    destroy = false
  }
}

removed {
  from = google_secret_manager_secret.openai_webhook_secret

  lifecycle {
    destroy = false
  }
}
