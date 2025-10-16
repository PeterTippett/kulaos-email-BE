# GCP Services and IAM Setup for Email Service

This document outlines the required Google Cloud Platform services and IAM permissions needed for the email service to function properly on App Engine.

## Required GCP Services

### 1. App Engine

- **Purpose**: Hosts the Go backend application
- **API**: `appengine.googleapis.com`
- **Configuration**: Defined in `app.yaml`

### 2. Cloud Datastore

- **Purpose**: Stores tenant and account data
- **API**: `datastore.googleapis.com`
- **Usage**: TenantStore and AccountStore in the application

### 3. BigQuery

- **Purpose**: Stores email data for analytics
- **API**: `bigquery.googleapis.com`
- **Usage**: BigQueryStore for email storage and querying

### 4. Cloud KMS (Key Management Service)

- **Purpose**: Encrypts sensitive data (OAuth tokens)
- **API**: `cloudkms.googleapis.com`
- **Usage**: KMSService for encryption/decryption

### 5. Pub/Sub

- **Purpose**: Handles Gmail webhook notifications
- **API**: `pubsub.googleapis.com`
- **Usage**: Gmail webhook processing

### 6. Gmail API

- **Purpose**: Access Gmail data and webhooks
- **API**: `gmail.googleapis.com`
- **Usage**: Gmail client operations

### 7. Cloud Scheduler

- **Purpose**: Automatically refreshes Gmail watch subscriptions
- **API**: `cloudscheduler.googleapis.com`
- **Usage**: Cron job for watch refresh

## Required IAM Roles

The App Engine default service account needs the following roles:

### Core Data Access

```bash
# Datastore access
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:PROJECT_ID@appspot.gserviceaccount.com" \
  --role="roles/datastore.user"

# BigQuery access
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:PROJECT_ID@appspot.gserviceaccount.com" \
  --role="roles/bigquery.dataEditor"

# KMS access
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:PROJECT_ID@appspot.gserviceaccount.com" \
  --role="roles/cloudkms.cryptoKeyEncrypterDecrypter"

# Pub/Sub access
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:PROJECT_ID@appspot.gserviceaccount.com" \
  --role="roles/pubsub.editor"
```

### Gmail API Access

```bash
# Gmail API access
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:PROJECT_ID@appspot.gserviceaccount.com" \
  --role="roles/gmail.readonly"
```

### Cloud Scheduler Access (for the scheduler service account)

```bash
# Create service account for Cloud Scheduler
gcloud iam service-accounts create gmail-watch-scheduler \
  --display-name="Gmail Watch Scheduler" \
  --description="Service account for Gmail watch refresh scheduler"

# Grant App Engine invoker role
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:gmail-watch-scheduler@PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/appengine.appViewer"
```

## Service Account Setup

### 1. App Engine Default Service Account

- **Account**: `PROJECT_ID@appspot.gserviceaccount.com`
- **Purpose**: Runs the application
- **Permissions**: All the roles listed above

### 2. Cloud Scheduler Service Account

- **Account**: `gmail-watch-scheduler@PROJECT_ID.iam.gserviceaccount.com`
- **Purpose**: Executes scheduled tasks
- **Permissions**: App Engine invoker

## Environment Variables Required

The following environment variables must be set in `app.yaml`:

### GCP Configuration

- `PROJECT_ID`: Your GCP project ID
- `KMS_LOCATION`: KMS key location (e.g., "global")
- `KMS_KEYRING`: KMS keyring name
- `BIGQUERY_LOCATION`: BigQuery dataset location

### Authentication

- `KINDE_DOMAIN`: Your Kinde domain
- `KINDE_CLIENT_ID`: Kinde OAuth client ID
- `KINDE_CLIENT_SECRET`: Kinde OAuth client secret
- `KINDE_REDIRECT_URI`: Kinde callback URL

### Gmail Integration

- `GMAIL_CLIENT_ID`: Gmail OAuth client ID
- `GMAIL_CLIENT_SECRET`: Gmail OAuth client secret
- `GMAIL_REDIRECT_URI`: Gmail callback URL

### Application

- `BACKEND_PORT`: "8080" (App Engine standard)
- `FRONTEND_BASE_URL`: Your frontend application URL
- `PUBSUB_TOPIC`: Pub/Sub topic name
- `ENVIRONMENT`: "production"

## Setup Commands

### 1. Enable All Required APIs

```bash
gcloud services enable appengine.googleapis.com
gcloud services enable datastore.googleapis.com
gcloud services enable bigquery.googleapis.com
gcloud services enable cloudkms.googleapis.com
gcloud services enable pubsub.googleapis.com
gcloud services enable gmail.googleapis.com
gcloud services enable cloudscheduler.googleapis.com
```

### 2. Create KMS Keyring and Key

```bash
# Create keyring
gcloud kms keyrings create email-service-keyring --location=global

# Create encryption key
gcloud kms keys create email-service-key \
  --keyring=email-service-keyring \
  --location=global \
  --purpose=encryption
```

### 3. Create BigQuery Dataset

```bash
# Create dataset
bq mk --location=australia-southeast1 email_service

# Create emails table
bq mk --table \
  email_service.emails \
  email_id:STRING,tenant_id:STRING,account_id:STRING,subject:STRING,body:TEXT,received_at:TIMESTAMP,labels:STRING
```

### 4. Create Pub/Sub Topic

```bash
# Create topic
gcloud pubsub topics create gmail-notifications

# Create subscription (optional, for testing)
gcloud pubsub subscriptions create gmail-notifications-sub \
  --topic=gmail-notifications
```

## Security Considerations

### 1. Service Account Keys

- **Never** download service account keys for App Engine
- Use the default service account with proper IAM roles
- Rotate keys regularly if using custom service accounts

### 2. Environment Variables

- Store sensitive values in `app.yaml` environment variables
- Use Cloud Secret Manager for highly sensitive data
- Never commit secrets to version control

### 3. Network Security

- App Engine provides automatic HTTPS
- Configure CORS properly for your frontend domain
- Use Cloud Armor for additional protection if needed

### 4. Data Encryption

- All data in transit is encrypted by default
- Use KMS for encrypting sensitive data at rest
- Enable audit logging for compliance

## Monitoring and Logging

### 1. Cloud Logging

- All application logs are automatically sent to Cloud Logging
- Use structured logging for better querying
- Set up log-based metrics and alerts

### 2. Cloud Monitoring

- Monitor App Engine metrics (requests, latency, errors)
- Set up alerts for application health
- Monitor GCP service quotas and usage

### 3. Error Reporting

- Enable Cloud Error Reporting for application errors
- Set up notifications for critical errors
- Use error patterns for debugging

## Cost Optimization

### 1. App Engine Scaling

- Configure appropriate min/max instances
- Use automatic scaling based on traffic
- Consider using basic scaling for predictable workloads

### 2. Resource Limits

- Set appropriate CPU and memory limits
- Monitor usage and adjust as needed
- Use Cloud Billing alerts to control costs

### 3. Data Storage

- Use appropriate storage classes for BigQuery
- Implement data retention policies
- Monitor Datastore usage and optimize queries

## Troubleshooting

### Common Issues

1. **Permission Denied Errors**

   - Check IAM roles are properly assigned
   - Verify service account has necessary permissions
   - Check API enablement status

2. **Authentication Failures**

   - Verify OAuth client configurations
   - Check redirect URIs match exactly
   - Ensure environment variables are set correctly

3. **Service Unavailable**
   - Check API quotas and limits
   - Verify all required services are enabled
   - Check application logs for specific errors

### Debugging Commands

```bash
# Check service account permissions
gcloud projects get-iam-policy PROJECT_ID

# View application logs
gcloud app logs tail -s default

# Check API status
gcloud services list --enabled

# Test API access
gcloud auth application-default print-access-token
```

## Next Steps

After completing the GCP setup:

1. **Deploy the application** using the deployment script
2. **Configure OAuth applications** with correct redirect URIs
3. **Set up Cloud Scheduler** for Gmail watch refresh
4. **Configure monitoring and alerting**
5. **Test all functionality** end-to-end
6. **Set up CI/CD pipeline** for automated deployments
