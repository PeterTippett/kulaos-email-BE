# Secret Management with GCP Secret Manager

This document explains how to securely manage sensitive configuration values for your email service using Google Cloud Platform Secret Manager.

## Overview

Instead of storing sensitive data like OAuth client secrets directly in `app.yaml` (which would be committed to version control), we use GCP Secret Manager to store these values securely and reference them in the App Engine configuration.

## Benefits of Using Secret Manager

1. **Security**: Secrets are encrypted at rest and in transit
2. **Version Control Safe**: No sensitive data in your repository
3. **Access Control**: Fine-grained IAM permissions
4. **Audit Trail**: Track who accessed what secrets when
5. **Rotation**: Easy secret rotation without code changes
6. **Centralized Management**: All secrets in one place

## Architecture

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   app.yaml     │    │  Secret Manager  │    │   App Engine    │
│                 │    │                  │    │                 │
│ Non-sensitive   │    │ Sensitive values │    │ Runtime access  │
│ values only     │───▶│ (OAuth secrets) │───▶│ via environment │
│                 │    │                  │    │ variables       │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

## Secret Manager Setup

### 1. Enable Secret Manager API

```bash
gcloud services enable secretmanager.googleapis.com
```

### 2. Create Secrets

Use the provided script to create all required secrets:

```bash
export PROJECT_ID="your-gcp-project-id"
./setup-secrets.sh
```

This script will:

- Enable the Secret Manager API
- Prompt you for sensitive values
- Create secrets in Secret Manager
- Grant App Engine access to the secrets

### 3. Manual Secret Creation

If you prefer to create secrets manually:

```bash
# Create Kinde secrets
echo -n "your-kinde-client-id" | gcloud secrets create kinde-client-id --data-file=-
echo -n "your-kinde-client-secret" | gcloud secrets create kinde-client-secret --data-file=-

# Create Gmail secrets
echo -n "your-gmail-client-id" | gcloud secrets create gmail-client-id --data-file=-
echo -n "your-gmail-client-secret" | gcloud secrets create gmail-client-secret --data-file=-

# Create Twilio secrets
echo -n "your-twilio-account-sid" | gcloud secrets create twilio-account-sid --data-file=-
echo -n "your-twilio-auth-token" | gcloud secrets create twilio-auth-token --data-file=-

# Create MCP secret (Model Context Protocol shared key for authentication)
echo -n "your-mcp-shared-key" | gcloud secrets create mcp-shared-key --data-file=-
```

## App Engine Configuration

### app.yaml Structure

Your `app.yaml` now has two sections:

1. **`env_variables`**: Non-sensitive configuration
2. **`secret_environment_variables`**: References to Secret Manager secrets

```yaml
runtime: go124

env_variables:
  # Non-sensitive values
  PROJECT_ID: "your-gcp-project-id"
  KINDE_DOMAIN: "your-tenant.kinde.com"
  FRONTEND_BASE_URL: "https://your-frontend-domain.com"
  # ... other non-sensitive values

secret_environment_variables:
  # Sensitive values from Secret Manager
  - key: KINDE_CLIENT_ID
    secret: kinde-client-id
  - key: KINDE_CLIENT_SECRET
    secret: kinde-client-secret
  - key: GMAIL_CLIENT_ID
    secret: gmail-client-id
  - key: GMAIL_CLIENT_SECRET
    secret: gmail-client-secret
  - key: TWILIO_ACCOUNT_SID
    secret: twilio-account-sid
  - key: TWILIO_AUTH_TOKEN
    secret: twilio-auth-token
  - key: MCP_SHARED_KEY
    secret: mcp-shared-key
```

## IAM Permissions

The App Engine default service account needs access to Secret Manager:

```bash
# Grant Secret Manager access
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:PROJECT_ID@appspot.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"
```

## Secret Management Commands

### List All Secrets

```bash
gcloud secrets list
```

### View Secret Value

```bash
# View latest version
gcloud secrets versions access latest --secret=SECRET_NAME

# View specific version
gcloud secrets versions access VERSION_NUMBER --secret=SECRET_NAME
```

### Update Secret Value

```bash
# Add new version (keeps old versions for rollback)
echo -n "new-secret-value" | gcloud secrets versions add SECRET_NAME --data-file=-
```

### Delete Secret

```bash
# Delete a secret (use with caution)
gcloud secrets delete SECRET_NAME
```

## Secret Rotation

### Automatic Rotation

Secret Manager supports automatic rotation using Cloud KMS:

```bash
# Enable automatic rotation (every 30 days)
gcloud secrets add-iam-policy-binding SECRET_NAME \
  --member="serviceAccount:rotation-service@PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretVersionManager"
```

### Manual Rotation Process

1. **Update the secret**:

   ```bash
   echo -n "new-client-secret" | gcloud secrets versions add kinde-client-secret --data-file=-
   ```

2. **Deploy the application** (App Engine will automatically use the latest version):

   ```bash
   gcloud app deploy app.yaml
   ```

3. **Verify the new secret is working**:

   ```bash
   curl https://your-app-id.appspot.com/health
   ```

4. **Clean up old versions** (optional):
   ```bash
   gcloud secrets versions destroy VERSION_NUMBER --secret=SECRET_NAME
   ```

## Security Best Practices

### 1. Least Privilege Access

Only grant the minimum required permissions:

```bash
# App Engine service account only needs secret accessor role
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:PROJECT_ID@appspot.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"
```

### 2. Audit Logging

Enable audit logging to track secret access:

```bash
# Enable audit logging for Secret Manager
gcloud logging sinks create secret-manager-audit \
  bigquery.googleapis.com/projects/PROJECT_ID/datasets/audit_logs \
  --log-filter='resource.type="secretmanager.googleapis.com/Secret"'
```

### 3. Secret Naming Convention

Use consistent naming patterns:

- `{service}-{type}-{environment}`
- Examples: `kinde-client-id-prod`, `gmail-client-secret-staging`

### 4. Environment Separation

Use different secrets for different environments:

```bash
# Production secrets
kinde-client-id-prod
kinde-client-secret-prod

# Staging secrets
kinde-client-id-staging
kinde-client-secret-staging
```

## Monitoring and Alerting

### Set Up Alerts

Create alerts for secret access anomalies:

```bash
# Create alerting policy for failed secret access
gcloud alpha monitoring policies create --policy-from-file=secret-access-policy.yaml
```

### Monitor Secret Usage

```bash
# View secret access logs
gcloud logging read 'resource.type="secretmanager.googleapis.com/Secret"' --limit=50
```

## Troubleshooting

### Common Issues

#### 1. Permission Denied

```bash
# Check if App Engine has access
gcloud secrets get-iam-policy SECRET_NAME

# Grant access if missing
gcloud secrets add-iam-policy-binding SECRET_NAME \
  --member="serviceAccount:PROJECT_ID@appspot.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"
```

#### 2. Secret Not Found

```bash
# List all secrets
gcloud secrets list

# Check if secret exists
gcloud secrets describe SECRET_NAME
```

#### 3. Application Can't Access Secret

- Verify the secret name in `app.yaml` matches the actual secret name
- Check IAM permissions
- Ensure Secret Manager API is enabled

### Debug Commands

```bash
# Check App Engine service account
gcloud app describe --format='value(defaultServiceAccount)'

# Verify secret access
gcloud secrets versions access latest --secret=SECRET_NAME

# Check IAM policy
gcloud secrets get-iam-policy SECRET_NAME
```

## Cost Considerations

### Secret Manager Pricing

- **Secret storage**: $0.06 per secret per month
- **Secret access**: $0.03 per 10,000 operations
- **Secret version**: $0.09 per version per month

### Cost Optimization

1. **Clean up old versions** regularly
2. **Use automatic rotation** to manage versions
3. **Monitor usage** with Cloud Billing alerts

## Migration from Environment Variables

If you're migrating from hardcoded secrets:

### 1. Create Secrets

```bash
# Create secrets from existing values
echo -n "$KINDE_CLIENT_SECRET" | gcloud secrets create kinde-client-secret --data-file=-
```

### 2. Update app.yaml

```yaml
# Remove from env_variables
# KINDE_CLIENT_SECRET: "hardcoded-value"

# Add to secret_environment_variables
secret_environment_variables:
  - key: KINDE_CLIENT_SECRET
    secret: kinde-client-secret
```

### 3. Deploy and Test

```bash
gcloud app deploy app.yaml
curl https://your-app-id.appspot.com/health
```

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: Deploy to App Engine
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2

      - name: Setup gcloud
        uses: google-github-actions/setup-gcloud@v0
        with:
          service_account_key: ${{ secrets.GCP_SA_KEY }}
          project_id: ${{ secrets.GCP_PROJECT_ID }}

      - name: Deploy to App Engine
        run: gcloud app deploy app.yaml
```

### Secret Access in CI/CD

```yaml
- name: Update secret
  run: |
    echo -n "${{ secrets.NEW_CLIENT_SECRET }}" | \
    gcloud secrets versions add kinde-client-secret --data-file=-
```

## Best Practices Summary

1. **Never commit secrets** to version control
2. **Use Secret Manager** for all sensitive configuration
3. **Implement least privilege** access
4. **Enable audit logging** for compliance
5. **Rotate secrets regularly** for security
6. **Monitor secret access** for anomalies
7. **Use environment-specific secrets** for separation
8. **Clean up old versions** to manage costs
9. **Test secret rotation** in staging first
10. **Document secret management** processes

## Next Steps

1. **Set up secrets** using `./setup-secrets.sh`
2. **Deploy your application** with `./deploy.sh`
3. **Configure monitoring** and alerting
4. **Implement secret rotation** strategy
5. **Train your team** on secret management practices
6. **Set up CI/CD** integration
7. **Regular security audits** of secret access
