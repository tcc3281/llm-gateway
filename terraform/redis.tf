# Google Memorystore Redis Instance
resource "google_redis_instance" "redis" {
  name               = "${var.app_name}-redis"
  tier               = "BASIC"
  memory_size_gb     = 1
  region             = var.region
  authorized_network = google_compute_network.vpc_network.id
  connect_mode       = "PRIVATE_SERVICE_ACCESS"

  redis_version = "REDIS_7_0"
  display_name  = "LLM Gateway Redis Cache & Rate Limiter"

  depends_on = [google_service_networking_connection.private_vpc_connection]
}
