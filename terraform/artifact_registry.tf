# Artifact Registry Repository for Docker Images
resource "google_artifact_registry_repository" "app_repo" {
  location      = var.region
  repository_id = "${var.app_name}-repo"
  description   = "Docker container repository for LLM Gateway"
  format        = "DOCKER"
}
