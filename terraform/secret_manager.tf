# Secret Manager Entries
resource "google_secret_manager_secret" "admin_api_key" {
  secret_id = "${var.app_name}-admin-api-key"

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "admin_api_key_ver" {
  secret      = google_secret_manager_secret.admin_api_key.id
  secret_data = var.admin_api_key
}

resource "google_secret_manager_secret" "db_password" {
  secret_id = "${var.app_name}-db-password"

  replication {
    auto {}
  }
}

resource "google_secret_manager_secret_version" "db_password_ver" {
  secret      = google_secret_manager_secret.db_password.id
  secret_data = var.db_password
}
