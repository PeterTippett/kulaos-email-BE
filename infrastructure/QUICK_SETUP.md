# Quick Setup Guide for Gmail Watch Refresh

This guide walks you through setting up the automated Gmail watch refresh system in **5 minutes**.

## Prerequisites

- GCP Project with billing enabled
- Backend service deployed (Cloud Run, GKE, or GCE)
- gcloud CLI installed and authenticated

## Quick Setup (gcloud CLI)

### 1. Navigate to Script

```bash
cd backend/infrastructure/gcloud
```

### 2. Edit Configuration

Open `setup-cloud-scheduler.sh` and update these variables:

```bash
PROJECT_ID="your-gcp-project-id"                      # Your GCP project
REGION="us-central1"                                   # Your preferred region
BACKEND_SERVICE_URL="https://your-backend-url"         # Your backend URL
```

For Cloud Run, find your backend URL:

```bash
gcloud run services describe your-service-name --region=us-central1 --format='value(status.url)'
```

### 3. Run Setup Script

```bash
chmod +x setup-cloud-scheduler.sh
./setup-cloud-scheduler.sh
```

### 4. Test the Job

```bash
gcloud scheduler jobs run gmail-watch-refresh --location=us-central1
```

### 5. Verify

Check the logs to confirm it worked:

```bash
# View recent logs
gcloud logging read "jsonPayload.message=~'watch subscription refresh'" --limit 10 --format json

# Check job status
gcloud scheduler jobs describe gmail-watch-refresh --location=us-central1
```

✅ **Done!** Your watch refresh is now automated.

---

## What Just Happened?

The setup script created:

1. **Service Account**: `gmail-watch-scheduler@PROJECT_ID.iam.gserviceaccount.com`

   - Used by Cloud Scheduler to authenticate requests

2. **Cloud Scheduler Job**: `gmail-watch-refresh`

   - Runs daily at 2:00 AM UTC
   - Calls your backend endpoint
   - Automatically retries on failure

3. **IAM Permissions**:
   - Service account can invoke your backend service

## Customization

### Change Schedule

Edit the schedule in the script using cron format:

```bash
--schedule="0 2 * * *"  # Daily at 2 AM UTC
--schedule="0 */6 * * *"  # Every 6 hours
--schedule="0 0 * * 0"  # Weekly on Sunday
```

### Change Timezone

```bash
--time-zone="America/New_York"  # Eastern Time
--time-zone="Europe/London"     # GMT/BST
--time-zone="UTC"               # Default
```

### View Available Timezones

```bash
gcloud scheduler time-zones list
```

## Monitoring

### View Recent Executions

```bash
gcloud scheduler jobs describe gmail-watch-refresh --location=us-central1
```

### View Logs

```bash
# Last 10 executions
gcloud logging read "resource.type=cloud_scheduler_job AND resource.labels.job_id=gmail-watch-refresh" --limit 10

# Watch subscription refresh logs
gcloud logging read "jsonPayload.message=~'watch subscription refresh'" --limit 50
```

### Manual Trigger

```bash
gcloud scheduler jobs run gmail-watch-refresh --location=us-central1
```

## Troubleshooting

### Job Not Running?

Check Cloud Scheduler API is enabled:

```bash
gcloud services enable cloudscheduler.googleapis.com
```

### Permission Denied?

For Cloud Run, ensure the service account has invoke permissions:

```bash
gcloud run services add-iam-policy-binding YOUR_SERVICE_NAME \
  --region=us-central1 \
  --member="serviceAccount:gmail-watch-scheduler@PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/run.invoker"
```

### Wrong Backend URL?

Update the job:

```bash
# Delete and recreate with correct URL
gcloud scheduler jobs delete gmail-watch-refresh --location=us-central1 --quiet
./setup-cloud-scheduler.sh
```

## Next Steps

1. **Set Up Monitoring**: Create alerts for job failures
2. **Review Logs**: Check that watches are being refreshed successfully
3. **Optimize Schedule**: Adjust frequency based on your needs
4. **Enable Audit Logs**: Track who/what triggers the job

## Getting Help

- 📖 See [GMAIL_WATCH_REFRESH.md](./GMAIL_WATCH_REFRESH.md) for full documentation
- 🐛 Check backend logs for detailed error messages
- 💬 Review Cloud Scheduler execution history in GCP Console

---

**Total Setup Time**: < 5 minutes  
**Ongoing Cost**: Free (within free tier)  
**Maintenance**: None (fully automated)
