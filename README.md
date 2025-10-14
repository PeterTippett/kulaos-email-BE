# Gmail Integration Backend

A Go-based backend service that processes Gmail emails with multi-tenant support, real-time push notifications, and secure storage in Google Cloud Platform.

## Overview

This backend service provides a complete email processing pipeline that:

- Authenticates users via Kinde (organization-based multi-tenancy)
- Connects Gmail accounts through OAuth 2.0
- Receives real-time Gmail push notifications via Google Cloud Pub/Sub
- Encrypts OAuth tokens using Google Cloud KMS (per-tenant encryption keys)
- Stores email data in isolated BigQuery datasets (one per tenant)
- Manages account metadata in Google Cloud Datastore with tenant namespacing

## Architecture

### Technology Stack

- **Language**: Go 1.21+
- **Authentication**: Kinde (JWT-based)
- **Storage**:
  - Google Cloud Datastore (account metadata, tenant-namespaced)
  - Google Cloud BigQuery (email data, per-tenant datasets)
- **Encryption**: Google Cloud KMS (tenant-specific encryption keys)
- **Messaging**: Google Cloud Pub/Sub (Gmail push notifications)
- **Email API**: Gmail API

### Data Flow

1. **User Authentication**: User logs in via Kinde, receives JWT token with organization ID
2. **Gmail Connection**: User initiates OAuth flow, grants Gmail permissions
3. **Token Encryption**: OAuth tokens encrypted with tenant-specific KMS key
4. **Token Storage**: Encrypted tokens stored in Datastore under tenant namespace
5. **Watch Setup**: Gmail watch notification registered for the connected account
6. **Email Reception**: Gmail sends push notification to Pub/Sub topic
7. **Webhook Processing**: Backend receives webhook, decrypts tokens, fetches email
8. **Storage**: Email stored in tenant-specific BigQuery dataset

### Multi-Tenancy Model

**Tenant Isolation Strategy:**

- Each Kinde organization represents a tenant
- Datastore uses namespaces (format: `tenant_{org_id}`)
- BigQuery creates separate datasets per tenant (format: `tenant_{org_id}_emails`)
- KMS encryption keys are tenant-specific (format: `tenant_{org_id}_key`)

**Security Features:**

- All OAuth tokens encrypted at rest using KMS
- Tenant data cannot cross namespace boundaries
- JWT validation ensures users only access their organization's data
- No shared storage between tenants

## Prerequisites

### Required Accounts & Services

1. **Google Cloud Platform**

   - Project with billing enabled
   - Service account with appropriate permissions
   - APIs enabled: Datastore, BigQuery, KMS, Pub/Sub, Gmail

2. **Kinde Account**

   - Application created and configured
   - At least one organization set up
   - Domain, Client ID, and Client Secret

3. **Gmail OAuth Credentials**
   - OAuth 2.0 Client ID (Web application type)
   - Redirect URI configured

### Required Tools

- **Go**: Version 1.21 or higher
- **Google Cloud SDK**: For `gcloud` CLI commands
- **ngrok** (for local development): To expose webhook endpoint

## Installation

### 1. Google Cloud Platform Setup

#### Enable Required APIs

```bash
gcloud services enable datastore.googleapis.com
gcloud services enable bigquery.googleapis.com
gcloud services enable cloudkms.googleapis.com
gcloud services enable pubsub.googleapis.com
gcloud services enable gmail.googleapis.com
```

#### Create KMS Keyring

```bash
gcloud kms keyrings create email-service-keyring --location=global
```

#### Create Pub/Sub Topic

```bash
gcloud pubsub topics create gmail-notifications
```

#### Set Up Service Account

```bash
# Create service account
gcloud iam service-accounts create email-service \
    --display-name="Email Service"

# Grant Datastore permissions
gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
    --member="serviceAccount:email-service@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
    --role="roles/datastore.user"

# Grant BigQuery permissions
gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
    --member="serviceAccount:email-service@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
    --role="roles/bigquery.admin"

# Grant KMS permissions
gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
    --member="serviceAccount:email-service@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
    --role="roles/cloudkms.cryptoKeyEncrypterDecrypter"

gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
    --member="serviceAccount:email-service@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
    --role="roles/cloudkms.admin"

# Download service account key
gcloud iam service-accounts keys create service-account-key.json \
    --iam-account=email-service@YOUR_PROJECT_ID.iam.gserviceaccount.com
```

#### Grant Gmail API Pub/Sub Publishing Permissions

```bash
gcloud pubsub topics add-iam-policy-binding gmail-notifications \
    --member=serviceAccount:gmail-api-push@system.gserviceaccount.com \
    --role=roles/pubsub.publisher \
    --project=YOUR_PROJECT_ID
```

### 2. Gmail OAuth Setup

1. Go to [Google Cloud Console - Credentials](https://console.cloud.google.com/apis/credentials)
2. Create OAuth 2.0 Client ID
   - Application type: **Web application**
   - Authorized redirect URI: `http://localhost:8085/auth/gmail/callback`
3. Enable Gmail API for your project
4. Copy the Client ID and Client Secret

### 3. Kinde Configuration

1. Create account at [kinde.com](https://kinde.com)
2. Create a new application
3. Configure callback URL: `http://localhost:3005/api/auth/kinde/callback`
4. Note your Domain, Client ID, and Client Secret
5. Create at least one organization in Kinde

### 4. Environment Configuration

Copy the example environment file:

```bash
cp env.example .env
```

Edit `.env` with your credentials:

```env
# GCP
PROJECT_ID=your-gcp-project-id
GOOGLE_APPLICATION_CREDENTIALS=./service-account-key.json

# KMS
KMS_LOCATION=global
KMS_KEYRING=email-service-keyring

# BigQuery
BIGQUERY_LOCATION=australia-southeast1

# Kinde
KINDE_DOMAIN=your-tenant.kinde.com
KINDE_CLIENT_ID=your-kinde-client-id
KINDE_CLIENT_SECRET=your-kinde-client-secret
KINDE_REDIRECT_URI=http://localhost:3005/api/auth/kinde/callback

# Gmail
GMAIL_CLIENT_ID=your-gmail-oauth-client-id
GMAIL_CLIENT_SECRET=your-gmail-oauth-secret
GMAIL_REDIRECT_URI=http://localhost:8085/auth/gmail/callback

# Backend
BACKEND_PORT=8085

# Pub/Sub
PUBSUB_TOPIC=gmail-notifications
```

### 5. Install Dependencies

Run the setup script:

```bash
chmod +x setup.sh
./setup.sh
```

Or manually:

```bash
go mod tidy
```

## Running the Server

### Start the Backend

```bash
go run cmd/server/main.go
```

The server will start on `http://localhost:8085`

### Expose Webhooks (Development)

Gmail push notifications require a public HTTPS endpoint. Use ngrok:

```bash
ngrok http 8085
```

Copy the HTTPS URL (e.g., `https://abc123.ngrok.io`)

### Configure Pub/Sub Subscription

Create a push subscription pointing to your ngrok URL:

```bash
gcloud pubsub subscriptions create gmail-notifications-sub \
  --topic=gmail-notifications \
  --push-endpoint=https://abc123.ngrok.io/webhooks/gmail
```

**Note**: Update this URL each time you restart ngrok (unless using a paid account with fixed domain)

## API Endpoints

### Authentication

All protected endpoints require a valid Kinde JWT token in the Authorization header:

```
Authorization: Bearer <kinde_jwt_token>
```

### Endpoints

#### `GET /health`

Health check endpoint

**Response:**

```json
{
  "status": "ok"
}
```

#### `POST /auth/gmail/init`

Initiates Gmail OAuth flow

**Headers:**

- `Authorization: Bearer <kinde_jwt_token>`

**Response:**

```json
{
  "url": "https://accounts.google.com/o/oauth2/auth?..."
}
```

#### `GET /auth/gmail/callback`

OAuth callback endpoint (handled by browser redirect)

**Query Parameters:**

- `code`: OAuth authorization code
- `state`: JWT token for authentication

**Response:**
Redirects to frontend with success/error status

#### `GET /accounts`

Lists all connected Gmail accounts for the authenticated user's organization

**Headers:**

- `Authorization: Bearer <kinde_jwt_token>`

**Response:**

```json
{
  "accounts": [
    {
      "email": "user@gmail.com",
      "created_at": "2024-01-15T10:30:00Z"
    }
  ]
}
```

#### `GET /emails`

Lists recent emails for the authenticated user's organization

**Headers:**

- `Authorization: Bearer <kinde_jwt_token>`

**Query Parameters:**

- `limit`: Maximum number of emails to return (default: 50)

**Response:**

```json
{
  "emails": [
    {
      "message_id": "abc123",
      "account_email": "user@gmail.com",
      "subject": "Test Email",
      "from": "sender@example.com",
      "to": "user@gmail.com",
      "received_at": "2024-01-15T14:22:00Z",
      "snippet": "This is a preview of the email..."
    }
  ]
}
```

#### `POST /webhooks/gmail`

Receives Gmail push notifications from Pub/Sub

**Headers:**

- Content-Type: `application/json`

**Body:**

```json
{
  "message": {
    "data": "base64-encoded-data",
    "messageId": "12345",
    "publishTime": "2024-01-15T14:22:00Z"
  }
}
```

## Project Structure

```
backend/
├── cmd/
│   └── server/
│       └── main.go              # Application entry point
├── internal/
│   ├── api/
│   │   └── handlers.go          # HTTP request handlers
│   ├── auth/
│   │   └── kinde.go             # Kinde JWT authentication middleware
│   ├── config/
│   │   └── config.go            # Configuration management
│   ├── datastore/
│   │   ├── account.go           # Email account operations
│   │   └── tenant.go            # Tenant management
│   ├── encryption/
│   │   └── kms.go               # KMS encryption/decryption
│   ├── gmail/
│   │   ├── client.go            # Gmail API client
│   │   ├── oauth.go             # OAuth 2.0 flow
│   │   └── webhook.go           # Webhook handler
│   ├── storage/
│   │   └── bigquery.go          # BigQuery operations
│   └── types/
│       └── email.go             # Data type definitions
├── env.example                   # Environment variables template
├── go.mod                        # Go module definition
├── go.sum                        # Go module checksums
├── service-account-key.json      # GCP service account key (gitignored)
└── README.md                     # This file
```

## Code Architecture

### Main Components

#### `cmd/server/main.go`

Application entry point that:

- Loads environment configuration
- Initializes GCP clients (Datastore, BigQuery, KMS, Pub/Sub)
- Sets up HTTP routes and middleware
- Starts the HTTP server

#### `internal/config/config.go`

Configuration management that loads and validates environment variables

#### `internal/auth/kinde.go`

JWT authentication middleware that:

- Validates Kinde JWT tokens
- Extracts user ID and organization ID
- Attaches tenant context to requests

#### `internal/datastore/account.go`

Gmail account management:

- `SaveAccount()`: Stores encrypted OAuth tokens
- `GetAccount()`: Retrieves and decrypts tokens
- `ListAccounts()`: Lists all accounts for a tenant

#### `internal/datastore/tenant.go`

Tenant operations:

- `EnsureTenant()`: Creates tenant namespace if needed
- Namespace formatting utilities

#### `internal/encryption/kms.go`

KMS encryption service:

- `Encrypt()`: Encrypts data with tenant-specific key
- `Decrypt()`: Decrypts data
- Automatic key creation per tenant

#### `internal/gmail/client.go`

Gmail API client:

- `FetchMessage()`: Retrieves email by ID
- `ParseEmailData()`: Extracts email metadata

#### `internal/gmail/oauth.go`

OAuth 2.0 flow:

- `GetAuthURL()`: Generates OAuth authorization URL
- `ExchangeCode()`: Exchanges code for tokens
- `SetupWatch()`: Registers Gmail push notifications

#### `internal/gmail/webhook.go`

Webhook processing:

- `HandleWebhook()`: Processes Pub/Sub notifications
- Fetches new emails
- Stores in BigQuery

#### `internal/storage/bigquery.go`

BigQuery operations:

- `EnsureDataset()`: Creates tenant dataset if needed
- `EnsureTable()`: Creates emails table if needed
- `InsertEmail()`: Stores email data
- `ListEmails()`: Queries emails for tenant

## Development

### Building

```bash
go build -o server cmd/server/main.go
```

### Running

```bash
./server
```

Or directly:

```bash
go run cmd/server/main.go
```

### Testing the Flow

1. Start the backend server
2. Start ngrok and configure Pub/Sub subscription
3. Use frontend or API client to authenticate via Kinde
4. Initiate Gmail OAuth flow with `/auth/gmail/init`
5. Complete OAuth and grant permissions
6. Send a test email to the connected Gmail account
7. Verify webhook receives notification (check logs)
8. Query emails via `/emails` endpoint
9. Verify data in BigQuery:

```bash
bq ls  # List datasets
bq query --use_legacy_sql=false 'SELECT * FROM `tenant_YOUR_ORG_ID_emails.emails` ORDER BY received_at DESC LIMIT 10'
```

## Troubleshooting

### Server Won't Start

**Problem**: Error loading configuration

**Solution**:

- Verify `.env` file exists and is properly formatted
- Check all required environment variables are set
- Ensure service account key file exists at specified path

**Problem**: GCP authentication fails

**Solution**:

- Verify `GOOGLE_APPLICATION_CREDENTIALS` points to valid service account key
- Ensure service account has necessary permissions
- Try running: `gcloud auth application-default login`

### OAuth Flow Fails

**Problem**: Redirect URI mismatch

**Solution**:

- Verify `GMAIL_REDIRECT_URI` in `.env` matches Google Cloud Console
- Check callback URL is `http://localhost:8085/auth/gmail/callback`

**Problem**: Invalid client credentials

**Solution**:

- Verify `GMAIL_CLIENT_ID` and `GMAIL_CLIENT_SECRET` are correct
- Ensure Gmail API is enabled in GCP project

### Webhooks Not Working

**Problem**: Not receiving push notifications

**Solution**:

- Verify ngrok is running and HTTPS URL is accessible
- Check Pub/Sub subscription push endpoint:
  ```bash
  gcloud pubsub subscriptions describe gmail-notifications-sub
  ```
- Verify Gmail API service account has publisher role on topic
- Check backend logs for webhook errors
- Test webhook manually:
  ```bash
  curl -X POST https://your-ngrok-url.ngrok.io/webhooks/gmail \
    -H "Content-Type: application/json" \
    -d '{"message": {"data": "", "messageId": "test"}}'
  ```

**Problem**: Watch registrations expiring

**Solution**:

- Gmail watch registrations expire after 7 days
- Re-connect Gmail account to renew watch
- For production, implement automatic renewal (not in POC)

### Data Storage Issues

**Problem**: Emails not appearing in BigQuery

**Solution**:

- Check backend logs for BigQuery errors
- Verify dataset exists: `bq ls`
- Verify table exists: `bq show tenant_YOUR_ORG_ID_emails.emails`
- Check service account has BigQuery admin role
- Query for errors in BigQuery job history

**Problem**: KMS encryption fails

**Solution**:

- Verify KMS keyring exists: `gcloud kms keyrings list --location=global`
- Check service account has KMS encrypter/decrypter role
- Ensure `KMS_LOCATION` and `KMS_KEYRING` match actual GCP resources

### Performance Optimization

For production deployments:

- Implement connection pooling for Datastore and BigQuery clients
- Add caching for frequently accessed account data
- Use batch BigQuery inserts for multiple emails
- Implement exponential backoff for API rate limiting
- Add request tracing and structured logging

## Security Considerations

### Token Encryption

- All OAuth tokens encrypted with KMS before storage
- Each tenant has isolated encryption key
- Keys never leave KMS (envelope encryption)

### Multi-Tenancy

- Datastore namespaces prevent cross-tenant data access
- BigQuery datasets are tenant-isolated
- JWT validation ensures organization context

### Production Checklist

- [ ] Use HTTPS for all endpoints (remove ngrok, use Cloud Run/App Engine)
- [ ] Implement proper CORS policies
- [ ] Add rate limiting per tenant
- [ ] Implement JWKS validation for Kinde tokens
- [ ] Use secret manager for credentials (not .env files)
- [ ] Enable audit logging
- [ ] Set up monitoring and alerting
- [ ] Implement OAuth token refresh
- [ ] Add webhook signature verification
- [ ] Configure VPC and firewall rules
- [ ] Use least-privilege IAM roles

## Production Deployment

### Recommended Platform

- **Google Cloud Run**: Serverless, auto-scaling, HTTPS by default
- **Alternative**: Google App Engine Standard

### Deployment Steps (Cloud Run)

1. Build container:

```bash
docker build -t gcr.io/YOUR_PROJECT_ID/email-backend .
```

2. Push to Container Registry:

```bash
docker push gcr.io/YOUR_PROJECT_ID/email-backend
```

3. Deploy to Cloud Run:

```bash
gcloud run deploy email-backend \
  --image gcr.io/YOUR_PROJECT_ID/email-backend \
  --platform managed \
  --region australia-southeast1 \
  --allow-unauthenticated \
  --set-env-vars="$(cat .env | xargs)"
```

4. Update Pub/Sub subscription with Cloud Run URL:

```bash
gcloud pubsub subscriptions update gmail-notifications-sub \
  --push-endpoint=https://your-cloud-run-url.run.app/webhooks/gmail
```

## Next Steps

This is a proof-of-concept implementation. For production:

1. **Implement OAuth Token Refresh**: Auto-refresh expired tokens
2. **Add Watch Renewal**: Auto-renew Gmail watch registrations (7-day expiry)
3. **Comprehensive Testing**: Unit tests, integration tests, load tests
4. **Monitoring & Logging**: Structured logging, metrics, traces, alerts
5. **Error Handling**: Retries, circuit breakers, graceful degradation
6. **API Documentation**: OpenAPI/Swagger specification
7. **CI/CD Pipeline**: Automated testing and deployment
8. **Webhook Security**: Verify Pub/Sub push request signatures
9. **Rate Limiting**: Per-tenant and per-user rate limits
10. **Data Retention**: Implement data lifecycle policies

## License

MIT
