output "session_image" {
  value       = "${local.image}:latest"
  description = "The session driver image the telephony layer deploys."
}
