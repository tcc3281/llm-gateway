variable "project_id" {
  type        = string
  description = "The GCP Project ID where resources will be created"
  default     = "gdgdevfest2026-515408"
}

variable "region" {
  type        = string
  description = "The GCP Region for resource deployment"
  default     = "asia-east1"
}

variable "credentials_file" {
  type        = string
  description = "Path to the GCP Service Account JSON key file relative to the terraform directory"
  default     = "gdgdevfest2026-515408-6b41e29434d7.json"
}

variable "app_name" {
  type        = string
  description = "Application name prefix for GCP resources"
  default     = "llm-gateway"
}

variable "environment" {
  type        = string
  description = "Deployment environment (production, staging, dev)"
  default     = "production"
}

variable "db_password" {
  type        = string
  description = "PostgreSQL Database Admin Password"
  sensitive   = true
  default     = "LLMGatewaySuperSecure2026!"
}

variable "admin_api_key" {
  type        = string
  description = "Master Admin API Key for LLM Gateway Management Server (:8081)"
  sensitive   = true
  default     = "sk-admin-secret-key-change-me"
}

variable "container_image" {
  type        = string
  description = "Docker Container Image URL for LLM Gateway"
  default     = "gcr.io/cloudrun/hello" # Placeholder until built & pushed to Artifact Registry
}
