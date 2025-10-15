# Gmail Watch Subscription Refresh

Gmail watch subscriptions expire after 7 days and must be renewed to continue receiving push notifications. This directory contains the automated renewal solution using Google Cloud Scheduler.

## Quick Start

For a quick 5-minute setup, see **[QUICK_SETUP.md](./QUICK_SETUP.md)**.

## How It Works

The automated refresh system:

1. **Runs daily at 2:00 AM UTC** via Cloud Scheduler
2. **Queries all active accounts** across all tenant namespaces
3. **Identifies expiring watches** (within 48 hours of expiry)
4. **Renews subscriptions** via Gmail API
5. **Updates database** with new expiration times

## Architecture

```
Cloud Scheduler (Daily 2 AM UTC)
        ↓
    OIDC Token Authentication
        ↓
Backend: POST /cron/refresh-watches
        ↓
1. Query Datastore → Get active accounts
2. Filter → Expiring within 48 hours
3. Refresh → Call Gmail API
4. Update → New expiration times
```

## Setup

### Prerequisites

- Backend deployed to GCP (Cloud Run, GKE, or GCE)
- gcloud CLI installed and authenticated
- Service account with appropriate permissions

### Installation

```bash
cd gcloud
# Edit setup-cloud-scheduler.sh with your configuration
./setup-cloud-scheduler.sh
```

### Testing

```bash
# Manually trigger the job
gcloud scheduler jobs run gmail-watch-refresh --location=us-central1

# View logs
gcloud logging read "jsonPayload.message=~'watch subscription refresh'" --limit 10
```

## Configuration

Edit `gcloud/setup-cloud-scheduler.sh`:

```bash
PROJECT_ID="your-gcp-project-id"
REGION="us-central1"
BACKEND_SERVICE_URL="https://your-backend-url"
```

### Customization

**Change Schedule:**

```bash
--schedule="0 2 * * *"    # Daily at 2 AM UTC (default)
--schedule="0 */6 * * *"  # Every 6 hours
```

**Change Timezone:**

```bash
--time-zone="UTC"               # Default
--time-zone="America/New_York"  # Eastern Time
```

## Monitoring

### View Job Status

```bash
gcloud scheduler jobs describe gmail-watch-refresh --location=us-central1
```

### Check Logs

```bash
# Scheduler job executions
gcloud logging read "resource.type=cloud_scheduler_job AND resource.labels.job_id=gmail-watch-refresh" --limit 10

# Refresh operation logs
gcloud logging read "jsonPayload.message=~'watch subscription refresh'" --limit 50
```

## What Was Created

The setup script creates:

1. **Service Account**: `gmail-watch-scheduler@PROJECT_ID.iam.gserviceaccount.com`

   - Used for Cloud Scheduler authentication

2. **Cloud Scheduler Job**: `gmail-watch-refresh`

   - Daily execution at 2:00 AM UTC
   - 3 retry attempts with exponential backoff
   - 5-minute timeout

3. **IAM Permissions**:
   - Service account granted invoke permissions on backend

## Troubleshooting

### Job Not Running

```bash
# Enable Cloud Scheduler API
gcloud services enable cloudscheduler.googleapis.com

# Verify job exists
gcloud scheduler jobs list --location=us-central1
```

### Permission Denied

For Cloud Run backends:

```bash
gcloud run services add-iam-policy-binding YOUR_SERVICE_NAME \
  --region=us-central1 \
  --member="serviceAccount:gmail-watch-scheduler@PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/run.invoker"
```

### Update Backend URL

```bash
# Delete old job
gcloud scheduler jobs delete gmail-watch-refresh --location=us-central1 --quiet

# Re-run setup with new URL
./setup-cloud-scheduler.sh
```

## Cost

**Estimated Monthly Cost: < $0.01**

- Cloud Scheduler: FREE (within free tier of 3 jobs)
- Job executions: FREE (within 250k free tier)
- Backend invocations: FREE (within Cloud Run free tier)

## Security

The system uses **OIDC token authentication**:

- Cloud Scheduler generates OIDC token with service account identity
- Backend validates token signature and issuer
- Only authorized service account can invoke endpoint
- No API keys or shared secrets required

## Architecture Diagrams

For detailed architecture diagrams and system flow, see **[ARCHITECTURE.md](./ARCHITECTURE.md)**.

## Complete Documentation

For comprehensive documentation including implementation details, see **[../../IMPLEMENTATION_SUMMARY.md](../../IMPLEMENTATION_SUMMARY.md)**.

## Files

```
infrastructure/
├── gcloud/
│   └── setup-cloud-scheduler.sh  # Setup script (executable)
├── ARCHITECTURE.md               # Detailed architecture diagrams
├── QUICK_SETUP.md               # 5-minute setup guide
└── GMAIL_WATCH_REFRESH.md       # This file
```

## Support

- 📖 Review [QUICK_SETUP.md](./QUICK_SETUP.md) for step-by-step instructions
- 🏗️ See [ARCHITECTURE.md](./ARCHITECTURE.md) for system design
- 🐛 Check backend logs for detailed error messages
- 💬 Review Cloud Scheduler execution history in GCP Console

---

**Setup Time**: < 5 minutes  
**Maintenance**: None (fully automated)  
**Reliability**: Auto-retry on failures
