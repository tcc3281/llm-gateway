data "google_project" "current" {
  project_id = var.project_id
}

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

# Grant Secret Accessor to Cloud Run Service Account
resource "google_secret_manager_secret_iam_member" "admin_api_key_access" {
  secret_id = google_secret_manager_secret.admin_api_key.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${data.google_project.current.number}-compute@developer.gserviceaccount.com"
}

resource "google_secret_manager_secret_iam_member" "db_password_access" {
  secret_id = google_secret_manager_secret.db_password.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${data.google_project.current.number}-compute@developer.gserviceaccount.com"
}
