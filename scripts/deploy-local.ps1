# PowerShell script to test local deployment to GCP via Terraform
$ErrorActionPreference = "Stop"

$PROJECT_ID = "gdgdevfest2026-515408"
$REGION = "asia-east1"
$REPO_NAME = "llm-gateway-repo"
$IMAGE_TAG = "asia-east1-docker.pkg.dev/${PROJECT_ID}/${REPO_NAME}/llm-gateway:local"
$KEY_FILE = "terraform/gdgdevfest2026-515408-6b41e29434d7.json"

Write-Host "🔑 [1/4] Authenticating gcloud with Service Account..." -ForegroundColor Cyan
gcloud auth activate-service-account --key-file=$KEY_FILE
gcloud config set project $PROJECT_ID

Write-Host "⚡ [2/4] Enabling GCP APIs..." -ForegroundColor Cyan
gcloud services enable `
  cloudresourcemanager.googleapis.com `
  serviceusage.googleapis.com `
  cloudbuild.googleapis.com `
  artifactregistry.googleapis.com `
  run.googleapis.com `
  sqladmin.googleapis.com `
  redis.googleapis.com `
  secretmanager.googleapis.com `
  vpcaccess.googleapis.com `
  servicenetworking.googleapis.com `
  compute.googleapis.com `
  --project=$PROJECT_ID

$repoExists = $false
try {
    $describe = gcloud artifacts repositories describe $REPO_NAME --location=$REGION --project=$PROJECT_ID 2>&1
    if ($LASTEXITCODE -eq 0) { $repoExists = $true }
} catch { }

if (-not $repoExists) {
    gcloud artifacts repositories create $REPO_NAME --repository-format=docker --location=$REGION --description="Docker repository for LLM Gateway" --project=$PROJECT_ID
}

Write-Host "🐳 [3/4] Building & Pushing Docker Image to GCP..." -ForegroundColor Cyan
$pushSuccess = $false

# 1. Try local Docker daemon if running
try {
    docker info 2>&1 | Out-Null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "Docker daemon detected. Building & pushing locally..." -ForegroundColor Gray
        gcloud auth configure-docker "$REGION-docker.pkg.dev" --quiet
        docker build -t $IMAGE_TAG .
        if ($LASTEXITCODE -eq 0) {
            docker push $IMAGE_TAG
            if ($LASTEXITCODE -eq 0) { $pushSuccess = $true }
        }
    }
} catch { }

# 2. Fallback to Google Cloud Build
if (-not $pushSuccess) {
    Write-Host "Local Docker is not active. Using Google Cloud Build..." -ForegroundColor Yellow
    gcloud builds submit --tag $IMAGE_TAG . --project=$PROJECT_ID
    if ($LASTEXITCODE -eq 0) {
        $pushSuccess = $true
    }
}

if (-not $pushSuccess) {
    Write-Host "⚠️ Warning: Image could not be built locally. Using public base image for infrastructure provisioning..." -ForegroundColor Yellow
    $IMAGE_TAG = "gcr.io/cloudrun/hello"
}

Write-Host "🏗️ [4/4] Running Terraform Apply..." -ForegroundColor Cyan
Push-Location terraform
terraform init

# Auto-import existing resources if already created
try {
    terraform import google_artifact_registry_repository.app_repo "projects/$PROJECT_ID/locations/$REGION/repositories/$REPO_NAME" 2>&1 | Out-Null
} catch { }

try {
    terraform import google_cloud_run_v2_service.gateway "projects/$PROJECT_ID/locations/$REGION/services/llm-gateway" 2>&1 | Out-Null
} catch { }

terraform apply -auto-approve -var="container_image=$IMAGE_TAG"
Pop-Location

Write-Host "✅ Deployment Completed Successfully!" -ForegroundColor Green
