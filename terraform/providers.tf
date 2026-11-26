terraform {
  required_version = ">= 1.5.0"

  backend "gcs" {
    bucket      = "gdgdevfest2026-515408-tfstate"
    prefix      = "llm-gateway/state"
    credentials = "gdgdevfest2026-515408-6b41e29434d7.json"
  }

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.20.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6.0"
    }
  }
}

provider "google" {
  project     = var.project_id
  region      = var.region
  credentials = fileexists("${path.module}/${var.credentials_file}") ? file("${path.module}/${var.credentials_file}") : null
}
