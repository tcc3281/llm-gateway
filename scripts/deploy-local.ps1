# PowerShell script to test local deployment to GCP via Terraform
$ErrorActionPreference = "Stop"

$PROJECT_ID = "gdgdevfest2026-515408"
$REGION = "asia-east1"
$REPO_NAME = "llm-gateway-repo"
$BUCKET_NAME = "gdgdevfest2026-515408-tfstate"
$KEY_FILE = "$PSScriptRoot\..\terraform\gdgdevfest2026-515408-6b41e29434d7.json"

if (Test-Path $KEY_FILE) {
    $env:GOOGLE_APPLICATION_CREDENTIALS = (Resolve-Path $KEY_FILE).Path
    $env:GOOGLE_CREDENTIALS = (Resolve-Path $KEY_FILE).Path
}

Write-Host "🔑 [1/4] Authenticating gcloud with Service Account..." -ForegroundColor Cyan
gcloud auth activate-service-account --key-file=$KEY_FILE
gcloud config set project $PROJECT_ID

Write-Host "⚡ [2/4] Enabling GCP APIs & Ensuring State Bucket exists..." -ForegroundColor Cyan
gcloud services enable `
  cloudresourcemanager.googleapis.com `
  serviceusage.googleapis.com `
  artifactregistry.googleapis.com `
  run.googleapis.com `
  sqladmin.googleapis.com `
  redis.googleapis.com `
  secretmanager.googleapis.com `
  vpcaccess.googleapis.com `
  servicenetworking.googleapis.com `
  compute.googleapis.com `
  storage.googleapis.com `
  --project=$PROJECT_ID

# Create state bucket if not exists
try {
    gcloud storage buckets describe "gs://$BUCKET_NAME" --project=$PROJECT_ID 2>&1 | Out-Null
} catch {
    gcloud storage buckets create "gs://$BUCKET_NAME" --location=$REGION --project=$PROJECT_ID --uniform-bucket-level-access
}

Write-Host "🏗️ [3/4] Migrating Terraform State to GCS..." -ForegroundColor Cyan
Push-Location "$PSScriptRoot\..\terraform"
terraform init -migrate-state -force-copy
Pop-Location

Write-Host "✅ Terraform State successfully synced to GCS!" -ForegroundColor Green
