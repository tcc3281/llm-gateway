#!/usr/bin/env bash
set -e

PROJECT_ID="gdgdevfest2026-515408"
REGION="asia-east1"
REPO_NAME="llm-gateway-repo"
IMAGE_TAG="asia-east1-docker.pkg.dev/${PROJECT_ID}/${REPO_NAME}/llm-gateway:local"
KEY_FILE="terraform/gdgdevfest2026-515408-6b41e29434d7.json"

echo "🔑 [1/5] Authenticating gcloud with Service Account..."
gcloud auth activate-service-account --key-file="$KEY_FILE"
gcloud config set project "$PROJECT_ID"

echo "⚡ [2/5] Enabling GCP APIs..."
gcloud services enable \
  cloudresourcemanager.googleapis.com \
  cloudbuild.googleapis.com \
  artifactregistry.googleapis.com \
  run.googleapis.com \
  sqladmin.googleapis.com \
  redis.googleapis.com \
  secretmanager.googleapis.com \
  vpcaccess.googleapis.com \
  servicenetworking.googleapis.com \
  compute.googleapis.com \
  --project="$PROJECT_ID"

gcloud artifacts repositories describe "$REPO_NAME" --location="$REGION" --project="$PROJECT_ID" || \
gcloud artifacts repositories create "$REPO_NAME" \
  --repository-format=docker \
  --location="$REGION" \
  --description="Docker repository for LLM Gateway" \
  --project="$PROJECT_ID"

echo "📦 [3/5] Building Web Admin UI..."
(cd web && npm run build)

echo "🐳 [4/5] Building & Pushing Docker Image to GCP..."
gcloud auth configure-docker "$REGION-docker.pkg.dev" --quiet
gcloud auth print-access-token | docker login -u oauth2accesstoken --password-stdin "https://$REGION-docker.pkg.dev" || true

if ! docker build -t "$IMAGE_TAG" . || ! docker push "$IMAGE_TAG"; then
    echo "Falling back to Google Cloud Build..."
    gcloud builds submit --tag "$IMAGE_TAG" . --project="$PROJECT_ID"
fi

echo "🏗️ [5/5] Running Terraform Apply..."
(
  cd terraform
  terraform init
  terraform import google_artifact_registry_repository.app_repo "projects/$PROJECT_ID/locations/$REGION/repositories/$REPO_NAME" || true
  terraform apply -auto-approve -var="container_image=$IMAGE_TAG"
)

echo "✅ Deployment Completed Successfully!"
