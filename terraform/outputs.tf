output "artifact_registry_repo" {
  description = "Docker Repository URL in GCP Artifact Registry"
  value       = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.app_repo.repository_id}"
}

output "cloud_run_url" {
  description = "Public URL of the deployed Cloud Run Gateway"
  value       = google_cloud_run_v2_service.gateway.uri
}

output "cloud_sql_private_ip" {
  description = "Private IP address of the Cloud SQL PostgreSQL instance"
  value       = google_sql_database_instance.postgres.private_ip_address
}

output "redis_private_ip" {
  description = "Private IP address of the Memorystore Redis instance"
  value       = google_redis_instance.redis.host
}
