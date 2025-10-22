# Email Service API Documentation

This document provides comprehensive documentation for all endpoints exposed by the Email Service backend.

## Base URL

- **Production**: `https://kulaos-email-prod.ts.r.appspot.com`
- **Development**: `http://localhost:8085`

## Authentication

Most endpoints require authentication via **Kinde JWT tokens**. Include the token in the `Authorization` header:

```
Authorization: Bearer <your-jwt-token>
```

## Rate Limiting

### General API Rate Limiting

- **Rate**: 10 requests per second per organization
- **Burst**: 20 requests
- **Scope**: Per-organization (isolated)
- **Response**: `429 Too Many Requests` when exceeded

### Email Send Rate Limiting

- **Rate**: 1 email per 5 seconds per organization
- **Scope**: Per-organization (isolated)
- **Response**: `429 Too Many Requests` with `Retry-After` header when exceeded
- **Note**: This is in addition to the general API rate limiting

## Endpoints Overview

| Method   | Path                        | Authentication | Description             |
| -------- | --------------------------- | -------------- | ----------------------- |
| `GET`    | `/health`                   | None           | Health check            |
| `GET`    | `/auth/gmail/start`         | Required       | Start Gmail OAuth flow  |
| `GET`    | `/auth/gmail/callback`      | None           | Gmail OAuth callback    |
| `GET`    | `/api/accounts`             | Required       | List connected accounts |
| `GET`    | `/api/emails`               | Required       | List recent emails      |
| `POST`   | `/api/emails/send`          | Required       | Send email              |
| `DELETE` | `/api/accounts/{accountID}` | Required       | Disconnect account      |
| `POST`   | `/webhooks/gmail`           | None           | Gmail webhook           |
| `POST`   | `/cron/refresh-watches`     | None           | Refresh Gmail watches   |

---

## Public Endpoints

### Health Check

**GET** `/health`

Check if the service is running and healthy.

#### Request

- **Method**: `GET`
- **Headers**: None required
- **Query Parameters**: None
- **Body**: None

#### Response

- **Status**: `200 OK`
- **Content-Type**: `application/json`

```json
{
  "status": "healthy"
}
```

---

### Gmail OAuth Callback

**GET** `/auth/gmail/callback`

Handles the OAuth callback from Google after user authorization.

#### Request

- **Method**: `GET`
- **Headers**: None required
- **Query Parameters**:
  - `code` (string, required): Authorization code from Google
  - `state` (string, required): State parameter containing org ID
- **Body**: None

#### Response

- **Status**: `302 Found` (redirect)
- **Location**: Redirects to frontend dashboard with success status

#### Example Request

```
GET /auth/gmail/callback?code=4/0AX4XfWh...&state=org_123:uuid-456
```

---

### Gmail Webhook

**POST** `/webhooks/gmail`

Receives Gmail push notifications via Google Cloud Pub/Sub.

#### Request

- **Method**: `POST`
- **Headers**:
  - `Content-Type: application/json`
- **Query Parameters**: None
- **Body**: Pub/Sub message format

```json
{
  "message": {
    "data": "eyJlbWFpbEFkZHJlc3MiOiJ0ZXN0QGV4YW1wbGUuY29tIiwiaGlzdG9yeUlkIjoxMjM0NX0=",
    "messageId": "123456789",
    "publishTime": "2024-01-15T10:30:00Z"
  },
  "subscription": "projects/project-id/subscriptions/gmail-notifications"
}
```

#### Response

- **Status**: `200 OK`
- **Body**: Empty

---

### Refresh Watch Subscriptions

**POST** `/cron/refresh-watches`

Refreshes expiring Gmail watch subscriptions. Called by Cloud Scheduler.

#### Request

- **Method**: `POST`
- **Headers**: None required
- **Query Parameters**: None
- **Body**: None

#### Response

- **Status**: `200 OK`
- **Content-Type**: `application/json`

```json
{
  "total_accounts": 5,
  "accounts_to_refresh": 2,
  "refreshed": 2,
  "failed": 0
}
```

---

## Protected Endpoints (Require Authentication)

### Start Gmail Authentication

**GET** `/auth/gmail/start`

Initiates the Gmail OAuth flow for the authenticated organization.

#### Request

- **Method**: `GET`
- **Headers**:
  - `Authorization: Bearer <jwt-token>`
- **Query Parameters**: None
- **Body**: None

#### Response

- **Status**: `200 OK`
- **Content-Type**: `application/json`

```json
{
  "auth_url": "https://accounts.google.com/o/oauth2/auth?client_id=...",
  "state": "org_123:uuid-456"
}
```

#### Error Responses

- **400 Bad Request**: Organization ID not found in token
- **500 Internal Server Error**: Failed to create tenant or generate auth URL

---

### List Connected Accounts

**GET** `/api/accounts`

Retrieves all Gmail accounts connected to the authenticated organization.

#### Request

- **Method**: `GET`
- **Headers**:
  - `Authorization: Bearer <jwt-token>`
- **Query Parameters**: None
- **Body**: None

#### Response

- **Status**: `200 OK`
- **Content-Type**: `application/json`

```json
{
  "accounts": [
    {
      "account_id": "550e8400-e29b-41d4-a716-446655440000",
      "email_address": "user@example.com",
      "status": "active",
      "created_at": "2024-01-15T10:30:00Z"
    },
    {
      "account_id": "550e8400-e29b-41d4-a716-446655440001",
      "email_address": "admin@company.com",
      "status": "active",
      "created_at": "2024-01-14T15:20:00Z"
    }
  ]
}
```

#### Error Responses

- **400 Bad Request**: Organization ID not found
- **500 Internal Server Error**: Failed to retrieve accounts

---

### List Recent Emails

**GET** `/api/emails`

Retrieves recent emails for the authenticated organization.

#### Request

- **Method**: `GET`
- **Headers**:
  - `Authorization: Bearer <jwt-token>`
- **Query Parameters**: None
- **Body**: None

#### Response

- **Status**: `200 OK`
- **Content-Type**: `application/json`

```json
{
  "emails": [
    {
      "MessageID": "18c2f4a5b6c7d8e9",
      "ThreadID": "18c2f4a5b6c7d8e9",
      "AccountID": "550e8400-e29b-41d4-a716-446655440000",
      "Sender": "sender@example.com",
      "SenderName": "John Doe",
      "Recipients": ["recipient@example.com"],
      "CCRecipients": ["cc@example.com"],
      "BCCRecipients": [],
      "Subject": "Meeting Tomorrow",
      "BodyText": "Hi, let's meet tomorrow at 2 PM.",
      "BodyHTML": "<p>Hi, let's meet tomorrow at 2 PM.</p>",
      "BodyMarkdown": "Hi, let's meet tomorrow at 2 PM.",
      "ReceivedAt": "2024-01-15T10:30:00Z",
      "IngestedAt": "2024-01-15T10:31:00Z",
      "IsRead": false,
      "Labels": ["INBOX", "UNREAD"]
    }
  ]
}
```

#### Error Responses

- **400 Bad Request**: Organization ID not found
- **500 Internal Server Error**: Failed to retrieve emails

---

### Send Email

**POST** `/api/emails/send`

Sends an email using a connected Gmail account.

#### Request

- **Method**: `POST`
- **Headers**:
  - `Authorization: Bearer <jwt-token>`
  - `Content-Type: application/json`
- **Query Parameters**: None
- **Body**: JSON object

```json
{
  "account_id": "550e8400-e29b-41d4-a716-446655440000",
  "to": "recipient@example.com",
  "subject": "Test Email",
  "body": "This is a test email sent via the API."
}
```

#### Request Body Schema

| Field        | Type   | Required | Description                          |
| ------------ | ------ | -------- | ------------------------------------ |
| `account_id` | string | Yes      | ID of the Gmail account to send from |
| `to`         | string | Yes      | Recipient email address              |
| `subject`    | string | Yes      | Email subject line                   |
| `body`       | string | Yes      | Email body content                   |

#### Response

- **Status**: `200 OK`
- **Content-Type**: `application/json`

```json
{
  "status": "success",
  "message": "Email sent successfully"
}
```

#### Error Responses

- **400 Bad Request**:
  - Missing required fields
  - Invalid email format
  - Organization ID not found
  - Account not found or not active
- **404 Not Found**: Account or tenant not found
- **429 Too Many Requests**:
  - Email send rate limit exceeded (1 email per 5 seconds)
  - Includes `Retry-After` header with seconds to wait
- **500 Internal Server Error**: Failed to send email

---

### Disconnect Account

**DELETE** `/api/accounts/{accountID}`

Disconnects a Gmail account from the organization.

#### Request

- **Method**: `DELETE`
- **Headers**:
  - `Authorization: Bearer <jwt-token>`
- **Query Parameters**: None
- **Path Parameters**:
  - `accountID` (string, required): ID of the account to disconnect
- **Body**: None

#### Response

- **Status**: `204 No Content`
- **Body**: Empty

#### Error Responses

- **400 Bad Request**:
  - Account ID is required
  - Organization ID not found
- **404 Not Found**: Account or tenant not found
- **500 Internal Server Error**: Failed to disconnect account

---

## Error Responses

All endpoints may return the following error responses:

### 400 Bad Request

```json
{
  "error": "Bad Request",
  "message": "Invalid request parameters"
}
```

### 401 Unauthorized

```json
{
  "error": "Unauthorized",
  "message": "Missing or invalid authentication token"
}
```

### 403 Forbidden

```json
{
  "error": "Forbidden",
  "message": "Insufficient permissions"
}
```

### 404 Not Found

```json
{
  "error": "Not Found",
  "message": "Resource not found"
}
```

### 429 Too Many Requests

**General API Rate Limit:**

```json
{
  "error": "Too Many Requests",
  "message": "Rate limit exceeded"
}
```

**Email Send Rate Limit:**

```json
{
  "error": "Too Many Requests",
  "message": "email send rate limit exceeded - please wait before sending another email"
}
```

_Headers: `Retry-After: 3` (seconds remaining)_

### 500 Internal Server Error

```json
{
  "error": "Internal Server Error",
  "message": "An unexpected error occurred"
}
```

---

## Data Types

### Account Object

```json
{
  "account_id": "string (UUID)",
  "email_address": "string (email)",
  "status": "string (active|disconnected)",
  "created_at": "string (ISO 8601 datetime)"
}
```

### Email Object

```json
{
  "MessageID": "string",
  "ThreadID": "string",
  "AccountID": "string (UUID)",
  "Sender": "string (email)",
  "SenderName": "string",
  "Recipients": ["string (email)"],
  "CCRecipients": ["string (email)"],
  "BCCRecipients": ["string (email)"],
  "Subject": "string",
  "BodyText": "string",
  "BodyHTML": "string",
  "BodyMarkdown": "string",
  "ReceivedAt": "string (ISO 8601 datetime)",
  "IngestedAt": "string (ISO 8601 datetime)",
  "IsRead": "boolean",
  "Labels": ["string"]
}
```

---

## Authentication Flow

1. **User Login**: User authenticates with Kinde
2. **Get Token**: Frontend receives JWT token from Kinde
3. **API Calls**: Include token in `Authorization: Bearer <token>` header
4. **Token Validation**: Backend validates token with Kinde JWKS
5. **Organization Context**: Backend extracts organization ID from token

## Gmail Integration Flow

1. **Start OAuth**: Call `/auth/gmail/start` to get authorization URL
2. **User Authorization**: User authorizes Gmail access in browser
3. **OAuth Callback**: Google redirects to `/auth/gmail/callback`
4. **Account Creation**: Backend creates/updates account with OAuth tokens
5. **Watch Setup**: Backend sets up Gmail push notifications
6. **Email Sync**: Gmail sends notifications to `/webhooks/gmail`

## Rate Limiting Details

### General API Rate Limiting

- **Algorithm**: Token bucket with sliding window
- **Scope**: Per-organization (isolated between organizations)
- **Storage**: In-memory (⚠️ **Note**: Not distributed across App Engine instances)
- **Headers**: No rate limit headers returned
- **Retry**: Clients should implement exponential backoff

### Email Send Rate Limiting

- **Algorithm**: Simple time-based (last sent timestamp)
- **Scope**: Per-organization (isolated between organizations)
- **Storage**: In-memory (⚠️ **Note**: Not distributed across App Engine instances)
- **Headers**: `Retry-After` header with seconds remaining
- **Retry**: Clients should wait for the time specified in `Retry-After` header

## CORS Configuration

- **Allowed Origins**: Configured frontend base URL only
- **Allowed Methods**: `GET`, `POST`, `PUT`, `DELETE`, `OPTIONS`
- **Allowed Headers**: `Content-Type`, `Authorization`
- **Credentials**: Supported

## Webhook Security

- **Authentication**: None (relies on Google Cloud Pub/Sub security)
- **Validation**: Pub/Sub message format validation
- **Retry Logic**: Automatic acknowledgment to prevent infinite retries
- **Rate Limiting**: Not applied to webhook endpoints

---

## Examples

### Complete Authentication Flow

```bash
# 1. Start Gmail OAuth
curl -X GET "https://kulaos-email-prod.ts.r.appspot.com/auth/gmail/start" \
  -H "Authorization: Bearer <jwt-token>"

# Response:
# {
#   "auth_url": "https://accounts.google.com/o/oauth2/auth?...",
#   "state": "org_123:uuid-456"
# }

# 2. User authorizes in browser (redirects to callback)

# 3. List accounts
curl -X GET "https://kulaos-email-prod.ts.r.appspot.com/api/accounts" \
  -H "Authorization: Bearer <jwt-token>"

# 4. Send email
curl -X POST "https://kulaos-email-prod.ts.r.appspot.com/api/emails/send" \
  -H "Authorization: Bearer <jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "account_id": "550e8400-e29b-41d4-a716-446655440000",
    "to": "recipient@example.com",
    "subject": "Test Email",
    "body": "Hello from the API!"
  }'
```

### Error Handling

```bash
# General API rate limit exceeded
curl -X GET "https://kulaos-email-prod.ts.r.appspot.com/api/accounts" \
  -H "Authorization: Bearer <jwt-token>"

# Response: 429 Too Many Requests
# {
#   "error": "Too Many Requests",
#   "message": "Rate limit exceeded"
# }

# Email send rate limit exceeded
curl -X POST "https://kulaos-email-prod.ts.r.appspot.com/api/emails/send" \
  -H "Authorization: Bearer <jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{"account_id": "...", "to": "...", "subject": "...", "body": "..."}'

# Response: 429 Too Many Requests
# Headers: Retry-After: 7
# {
#   "error": "Too Many Requests",
#   "message": "email send rate limit exceeded - please wait before sending another email"
# }
```

---

## Notes

- All timestamps are in ISO 8601 format (UTC)
- Email addresses are normalized to lowercase
- Account IDs are UUIDs
- The service automatically handles Gmail token refresh
- Webhook processing is asynchronous and may have delays
- Rate limiting is per-organization, not per-user
- All sensitive data (OAuth tokens) is encrypted at rest using Google Cloud KMS
