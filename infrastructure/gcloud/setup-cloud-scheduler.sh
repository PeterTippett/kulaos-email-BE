#!/bin/bash
# Script to set up Cloud Scheduler for Gmail watch refresh using gcloud CLI
# This is an alternative to using Terraform

set -e

# Configuration - Update these values
PROJECT_ID="kulaos-email-prod"
REGION="australia-southeast1"
BACKEND_SERVICE_URL="https://kulaos-email-prod.ts.r.appspot.com"
SERVICE_ACCOUNT_NAME="gmail-watch-scheduler"
JOB_NAME="gmail-watch-refresh"

echo "Setting up Cloud Scheduler for Gmail watch refresh..."

# Set the project
gcloud config set project $PROJECT_ID

# Create service account if it doesn't exist
if ! gcloud iam service-accounts describe ${SERVICE_ACCOUNT_NAME}@${PROJECT_ID}.iam.gserviceaccount.com &>/dev/null; then
  echo "Creating service account: ${SERVICE_ACCOUNT_NAME}"
  gcloud iam service-accounts create ${SERVICE_ACCOUNT_NAME} \
    --display-name="Gmail Watch Scheduler Service Account" \
    --description="Service account used by Cloud Scheduler to invoke watch refresh endpoint"
else
  echo "Service account ${SERVICE_ACCOUNT_NAME} already exists"
fi

SERVICE_ACCOUNT_EMAIL="${SERVICE_ACCOUNT_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"

# If using Cloud Run, grant the service account permission to invoke the service
# Uncomment and adjust the following lines if your backend is deployed to Cloud Run:
# CLOUD_RUN_SERVICE_NAME="your-cloud-run-service-name"
# gcloud run services add-iam-policy-binding $CLOUD_RUN_SERVICE_NAME \
#   --region=$REGION \
#   --member="serviceAccount:${SERVICE_ACCOUNT_EMAIL}" \
#   --role="roles/run.invoker"

# Delete existing job if it exists (to update configuration)
if gcloud scheduler jobs describe $JOB_NAME --location=$REGION &>/dev/null; then
  echo "Deleting existing Cloud Scheduler job: ${JOB_NAME}"
  gcloud scheduler jobs delete $JOB_NAME --location=$REGION --quiet
fi

# Create Cloud Scheduler job
echo "Creating Cloud Scheduler job: ${JOB_NAME}"
gcloud scheduler jobs create http $JOB_NAME \
  --location=$REGION \
  --schedule="0 2 * * *" \
  --time-zone="Australia/Sydney" \
  --uri="${BACKEND_SERVICE_URL}/cron/refresh-watches" \
  --http-method=POST \
  --oidc-service-account-email=$SERVICE_ACCOUNT_EMAIL \
  --oidc-token-audience="${BACKEND_SERVICE_URL}" \
  --headers="Content-Type=application/json" \
  --attempt-deadline="320s" \
  --max-retry-attempts=3 \
  --min-backoff=5s \
  --max-backoff=3600s \
  --max-doublings=5

echo "✅ Cloud Scheduler job created successfully!"
echo ""
echo "Job details:"
echo "  Name: ${JOB_NAME}"
echo "  Schedule: Daily at 2:00 AM Australia/Sydney"
echo "  Endpoint: ${BACKEND_SERVICE_URL}/cron/refresh-watches"
echo "  Service Account: ${SERVICE_ACCOUNT_EMAIL}"
echo ""
echo "To manually trigger the job for testing:"
echo "  gcloud scheduler jobs run $JOB_NAME --location=$REGION"
echo ""
echo "To view job details:"
echo "  gcloud scheduler jobs describe $JOB_NAME --location=$REGION"

