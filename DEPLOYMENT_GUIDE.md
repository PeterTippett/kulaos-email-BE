# GCP App Engine Deployment Guide

This guide walks you through deploying your Go email service to Google Cloud Platform App Engine.

## Prerequisites

Before starting, ensure you have:

1. **GCP Project** with billing enabled
2. **gcloud CLI** installed and authenticated
3. **Kinde OAuth** application configured
4. **Gmail OAuth** application configured
5. **Domain** for your frontend application

## Step 1: Initial Setup

### 1.1 Install and Configure gcloud CLI

```bash
# Install gcloud CLI (if not already installed)
# Visit: https://cloud.google.com/sdk/docs/install

# Authenticate
gcloud auth login

# Set your project
export PROJECT_ID="your-gcp-project-id"
gcloud config set project $PROJECT_ID
```

### 1.2 Clone and Navigate to Backend

```bash
cd /Users/oldo/repos/HelloAgain/kula/emails/backend
```

## Step 2: Configure Environment Variables

### 2.1 Update app.yaml

Edit the `app.yaml` file and replace all placeholder values:

```yaml
# Update these values in app.yaml
env_variables:
  PROJECT_ID: "your-actual-gcp-project-id"
  KINDE_DOMAIN: "your-actual-kinde-domain.kinde.com"
  KINDE_CLIENT_ID: "your-actual-kinde-client-id"
  KINDE_CLIENT_SECRET: "your-actual-kinde-client-secret"
  GMAIL_CLIENT_ID: "your-actual-gmail-oauth-client-id"
  GMAIL_CLIENT_SECRET: "your-actual-gmail-oauth-secret"
  FRONTEND_BASE_URL: "https://your-actual-frontend-domain.com"
  # ... other variables
```

### 2.2 Update OAuth Redirect URIs

After deployment, you'll need to update your OAuth applications with the App Engine URLs:

- **Kinde Redirect URI**: `https://your-app-id.appspot.com/api/auth/kinde/callback`
- **Gmail Redirect URI**: `https://your-app-id.appspot.com/auth/gmail/callback`

## Step 3: Set Up GCP Services

### 3.1 Enable Required APIs

```bash
# Run the API enablement commands
gcloud services enable appengine.googleapis.com
gcloud services enable datastore.googleapis.com
gcloud services enable bigquery.googleapis.com
gcloud services enable cloudkms.googleapis.com
gcloud services enable pubsub.googleapis.com
gcloud services enable gmail.googleapis.com
gcloud services enable cloudscheduler.googleapis.com
```

### 3.2 Create KMS Resources

```bash
# Create KMS keyring
gcloud kms keyrings create email-service-keyring --location=global

# Create encryption key
gcloud kms keys create email-service-key \
  --keyring=email-service-keyring \
  --location=global \
  --purpose=encryption
```

### 3.3 Create BigQuery Dataset

```bash
# Create dataset
bq mk --location=australia-southeast1 email_service

# Create emails table
bq mk --table \
  email_service.emails \
  email_id:STRING,tenant_id:STRING,account_id:STRING,subject:STRING,body:TEXT,received_at:TIMESTAMP,labels:STRING
```

### 3.4 Create Pub/Sub Topic

```bash
# Create topic
gcloud pubsub topics create gmail-notifications
```

## Step 4: Configure IAM Permissions

### 4.1 Grant App Engine Service Account Permissions

```bash
# Get your project ID
PROJECT_ID=$(gcloud config get-value project)

# Grant Datastore access
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:$PROJECT_ID@appspot.gserviceaccount.com" \
  --role="roles/datastore.user"

# Grant BigQuery access
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:$PROJECT_ID@appspot.gserviceaccount.com" \
  --role="roles/bigquery.dataEditor"

# Grant KMS access
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:$PROJECT_ID@appspot.gserviceaccount.com" \
  --role="roles/cloudkms.cryptoKeyEncrypterDecrypter"

# Grant Pub/Sub access
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:$PROJECT_ID@appspot.gserviceaccount.com" \
  --role="roles/pubsub.editor"

# Grant Gmail API access
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:$PROJECT_ID@appspot.gserviceaccount.com" \
  --role="roles/gmail.readonly"
```

## Step 5: Deploy the Application

### 5.1 Run the Deployment Script

```bash
# Make the script executable (if not already done)
chmod +x deploy.sh

# Set your project ID
export PROJECT_ID="your-gcp-project-id"

# Run the deployment
./deploy.sh
```

### 5.2 Manual Deployment (Alternative)

If you prefer to deploy manually:

```bash
# Create App Engine app (if not exists)
gcloud app create --region=australia-southeast1

# Deploy the application
gcloud app deploy app.yaml
```

## Step 6: Post-Deployment Configuration

### 6.1 Get Your App URL

```bash
# Get the deployed URL
gcloud app browse --no-launch-browser
```

### 6.2 Update OAuth Applications

Update your OAuth applications with the new URLs:

1. **Kinde Dashboard**:

   - Go to your Kinde application settings
   - Update redirect URI to: `https://your-app-id.appspot.com/api/auth/kinde/callback`

2. **Google Cloud Console**:
   - Go to APIs & Services > Credentials
   - Update your Gmail OAuth client redirect URI to: `https://your-app-id.appspot.com/auth/gmail/callback`

### 6.3 Update Frontend Configuration

Update your frontend application to use the new backend URL:

```typescript
// In your frontend configuration
const API_BASE_URL = "https://your-app-id.appspot.com";
```

## Step 7: Set Up Cloud Scheduler (Optional)

### 7.1 Create Scheduler Service Account

```bash
# Create service account for Cloud Scheduler
gcloud iam service-accounts create gmail-watch-scheduler \
  --display-name="Gmail Watch Scheduler" \
  --description="Service account for Gmail watch refresh scheduler"

# Grant App Engine invoker role
gcloud projects add-iam-policy-binding $PROJECT_ID \
  --member="serviceAccount:gmail-watch-scheduler@$PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/appengine.appViewer"
```

### 7.2 Set Up Scheduled Job

```bash
# Navigate to the scheduler script
cd infrastructure/gcloud

# Edit the script with your values
# Update PROJECT_ID, REGION, and BACKEND_SERVICE_URL

# Run the setup
chmod +x setup-cloud-scheduler.sh
./setup-cloud-scheduler.sh
```

## Step 8: Testing and Verification

### 8.1 Health Check

```bash
# Test the health endpoint
curl https://your-app-id.appspot.com/health
```

Expected response:

```json
{ "status": "healthy", "timestamp": "2024-01-01T00:00:00Z" }
```

### 8.2 Test Authentication Flow

1. Visit your frontend application
2. Try to connect Gmail
3. Verify the OAuth flow works
4. Check that accounts are created in Datastore

### 8.3 Monitor Logs

```bash
# View application logs
gcloud app logs tail -s default

# View specific log entries
gcloud logging read "resource.type=gae_app" --limit=50
```

## Step 9: Production Considerations

### 9.1 Security

- **HTTPS**: App Engine provides automatic HTTPS
- **CORS**: Configured to allow your frontend domain
- **Secrets**: Store sensitive data in environment variables
- **IAM**: Use least-privilege access

### 9.2 Monitoring

- **Cloud Logging**: All logs are automatically collected
- **Cloud Monitoring**: Set up alerts for errors and performance
- **Error Reporting**: Enable for application error tracking

### 9.3 Scaling

- **Automatic Scaling**: Configured in app.yaml
- **Resource Limits**: Set appropriate CPU and memory limits
- **Cost Optimization**: Monitor usage and adjust scaling parameters

## Troubleshooting

### Common Issues

#### 1. Permission Denied Errors

```bash
# Check service account permissions
gcloud projects get-iam-policy $PROJECT_ID

# Verify API enablement
gcloud services list --enabled | grep -E "(appengine|datastore|bigquery|kms|pubsub|gmail)"
```

#### 2. OAuth Errors

- Verify redirect URIs match exactly
- Check client IDs and secrets are correct
- Ensure OAuth consent screen is configured

#### 3. Service Unavailable

- Check API quotas and limits
- Verify all required services are enabled
- Review application logs for specific errors

#### 4. Configuration Errors

- Validate app.yaml syntax
- Check environment variables are set
- Verify GCP project ID is correct

### Debugging Commands

```bash
# Check application status
gcloud app describe

# View recent logs
gcloud app logs tail -s default --num-logs=100

# Check service account
gcloud iam service-accounts describe $PROJECT_ID@appspot.gserviceaccount.com

# Test API access
gcloud auth application-default print-access-token
```

## Maintenance

### Regular Tasks

1. **Monitor Costs**: Check Cloud Billing dashboard regularly
2. **Review Logs**: Look for errors or performance issues
3. **Update Dependencies**: Keep Go modules updated
4. **Security Updates**: Apply security patches promptly
5. **Backup Data**: Ensure Datastore and BigQuery data is backed up

### Scaling Considerations

- **Traffic Growth**: Monitor request volume and adjust scaling
- **Data Growth**: Monitor Datastore and BigQuery usage
- **Cost Optimization**: Review resource usage and optimize

## Support and Resources

- **GCP Documentation**: https://cloud.google.com/appengine/docs
- **App Engine Go Runtime**: https://cloud.google.com/appengine/docs/go/
- **Cloud Logging**: https://cloud.google.com/logging/docs
- **Cloud Monitoring**: https://cloud.google.com/monitoring/docs

## Next Steps

After successful deployment:

1. **Set up CI/CD pipeline** for automated deployments
2. **Configure monitoring and alerting**
3. **Implement backup strategies**
4. **Set up staging environment**
5. **Plan for disaster recovery**

---

**Deployment Checklist:**

- [ ] GCP project created and billing enabled
- [ ] gcloud CLI installed and authenticated
- [ ] All required APIs enabled
- [ ] KMS keyring and key created
- [ ] BigQuery dataset created
- [ ] Pub/Sub topic created
- [ ] IAM permissions configured
- [ ] app.yaml configured with actual values
- [ ] Application deployed successfully
- [ ] OAuth applications updated
- [ ] Frontend configured with new backend URL
- [ ] Health check passing
- [ ] Authentication flow tested
- [ ] Cloud Scheduler configured (optional)
- [ ] Monitoring and logging verified
