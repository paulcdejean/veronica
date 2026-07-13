# The call handoff: the webhook publishes {call_id, caller} here and
# scales the session pool up; the starting driver pulls its call from the
# subscription. Retention is the minimum — a call is only answerable while
# the phone rings, so anything old is garbage the driver drops via the
# accept's 404.
resource "google_pubsub_topic" "calls" {
  name       = "${tofu.workspace}-calls"
  depends_on = [google_project_service.services]
}

resource "google_pubsub_subscription" "calls" {
  name  = "${tofu.workspace}-calls"
  topic = google_pubsub_topic.calls.id

  # 600s is the API minimum retention. The ack deadline covers the driver's
  # pull-to-accept window; an unacked (crashed-on) call redelivers after it.
  message_retention_duration = "600s"
  ack_deadline_seconds       = 30

  expiration_policy {
    ttl = "" # never expire; weeks of idle are normal for a personal line
  }
}
