variable "custom_domain" {
  type        = string
  description = "Custom domain name for LLM Gateway"
  default     = "vin-ai-t205.io.vn"
}

variable "enable_custom_domain" {
  type        = bool
  description = "Enable Cloud Run domain mapping (set to true after domain ownership is verified in GCP Search Console)"
  default     = false
}

# Cloud Run Custom Domain Mapping
resource "google_cloud_run_domain_mapping" "custom_domain" {
  count    = var.enable_custom_domain ? 1 : 0
  location = var.region
  name     = var.custom_domain

  metadata {
    namespace = var.project_id
  }

  spec {
    route_name = google_cloud_run_v2_service.gateway.name
  }
}

output "dns_resource_records" {
  description = "DNS records to configure at your domain registrar for HTTPS/SSL"
  value       = var.enable_custom_domain ? google_cloud_run_domain_mapping.custom_domain[0].status[0].resource_records : []
}
