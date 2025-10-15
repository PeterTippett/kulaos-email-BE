# Gmail Watch Refresh - Architecture Diagram

## System Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          Gmail Watch Lifecycle                           │
│                                                                          │
│  Initial Setup              Auto Refresh              Expiration         │
│  ─────────────              ────────────              ──────────         │
│                                                                          │
│  User connects      Cloud Scheduler runs     Watch subscription         │
│  Gmail account  →   daily to check for   →   expires after 7 days       │
│                     watches expiring                                     │
│                     within 48 hours                                      │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

## Component Architecture

```
┌──────────────────────────────────────────────────────────────────────────┐
│                              GCP Cloud                                    │
│                                                                           │
│  ┌─────────────────────┐                                                 │
│  │  Cloud Scheduler    │                                                 │
│  │  ─────────────────  │                                                 │
│  │  Cron: 0 2 * * *   │  1. Trigger daily                               │
│  │  (2:00 AM UTC)      │─────────────────┐                              │
│  └─────────────────────┘                 │                              │
│                                           ▼                              │
│  ┌─────────────────────────────────────────────────────────────┐        │
│  │              Backend Service (Cloud Run/GKE/GCE)            │        │
│  │  ─────────────────────────────────────────────────────────  │        │
│  │                                                              │        │
│  │  2. Receive request                                         │        │
│  │     POST /cron/refresh-watches                              │        │
│  │     Authorization: Bearer <OIDC_TOKEN>                      │        │
│  │                                                              │        │
│  │  ┌────────────────────────────────────────────────┐         │        │
│  │  │  CloudSchedulerAuth Middleware                 │         │        │
│  │  │  ─────────────────────────────────────────     │         │        │
│  │  │  • Validate OIDC token                         │         │        │
│  │  │  • Verify issuer (Google)                      │         │        │
│  │  │  • Check service account email                 │         │        │
│  │  └────────────────────────────────────────────────┘         │        │
│  │                         │                                    │        │
│  │                         ▼                                    │        │
│  │  ┌────────────────────────────────────────────────┐         │        │
│  │  │  RefreshWatchSubscriptions Handler             │         │        │
│  │  │  ────────────────────────────────────────      │         │        │
│  │  │  3. Query all active accounts                  │         │        │
│  │  │  4. Filter expiring watches (< 48h)            │         │        │
│  │  │  5. For each account:                          │         │        │
│  │  │     • Decrypt OAuth tokens (KMS)               │         │        │
│  │  │     • Call Gmail API                           │         │        │
│  │  │     • Update Datastore                         │         │        │
│  │  └────────────────────────────────────────────────┘         │        │
│  │                         │                                    │        │
│  └─────────────────────────┼────────────────────────────────────┘        │
│                            │                                             │
│         ┌──────────────────┼──────────────────┐                          │
│         │                  │                  │                          │
│         ▼                  ▼                  ▼                          │
│  ┌────────────┐    ┌────────────┐    ┌────────────┐                     │
│  │ Datastore  │    │    KMS     │    │ Gmail API  │                     │
│  │ ──────────│    │ ──────────│    │ ──────────│                     │
│  │ • Accounts │    │ • Decrypt  │    │ • Create   │                     │
│  │ • Tenants  │    │   tokens   │    │   watch    │                     │
│  │ • Query    │    │            │    │            │                     │
│  │ • Update   │    │            │    │            │                     │
│  └────────────┘    └────────────┘    └────────────┘                     │
│                                                                           │
└──────────────────────────────────────────────────────────────────────────┘
```

## Data Flow

```
┌─────────────┐
│   Start     │
└──────┬──────┘
       │
       ▼
┌─────────────────────────────────────────┐
│ Cloud Scheduler triggers at 2:00 AM UTC │
└──────┬──────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────┐
│ Generate OIDC token for service     │
│ account authentication              │
└──────┬──────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────┐
│ POST /cron/refresh-watches          │
│ with OIDC token                     │
└──────┬──────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────┐
│ Middleware: Validate OIDC token     │
│ ✓ Issuer = accounts.google.com     │
│ ✓ Email = scheduler service account│
│ ✓ Not expired                       │
└──────┬──────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────┐
│ Query Datastore:                    │
│ GetAllActiveAccounts()              │
│ - Iterate all tenant namespaces     │
│ - Filter status = "active"          │
└──────┬──────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────┐
│ Filter accounts:                    │
│ - webhook_expiration is null  OR    │
│ - webhook_expiration < now + 48h    │
└──────┬──────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────┐
│ For each filtered account:          │
│ ┌─────────────────────────────────┐ │
│ │ 1. Get tenant namespace         │ │
│ │ 2. Decrypt OAuth tokens (KMS)   │ │
│ │ 3. Create Gmail service client  │ │
│ │ 4. Call users.watch() API       │ │
│ │ 5. Parse response:              │ │
│ │    • historyId                  │ │
│ │    • expiration (timestamp)     │ │
│ │ 6. Update Datastore:            │ │
│ │    • webhook_channel_id         │ │
│ │    • webhook_expiration         │ │
│ │    • updated_at                 │ │
│ └─────────────────────────────────┘ │
└──────┬──────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────┐
│ Return JSON response:               │
│ {                                   │
│   "total_accounts": 10,             │
│   "accounts_to_refresh": 3,         │
│   "refreshed": 3,                   │
│   "failed": 0,                      │
│   "errors": []                      │
│ }                                   │
└──────┬──────────────────────────────┘
       │
       ▼
┌─────────────┐
│  Complete   │
└─────────────┘
```

## Multi-Tenant Data Isolation

```
┌───────────────────────────────────────────────────────────┐
│                    Datastore Namespaces                    │
│                                                            │
│  ┌──────────────────┐  ┌──────────────────┐              │
│  │ DEFAULT          │  │ tenant_org123    │              │
│  │ ──────────────── │  │ ──────────────── │              │
│  │ • Tenant info    │  │ • EmailAccount   │              │
│  │ • Email lookups  │  │   - account_id   │              │
│  │                  │  │   - email_addr   │              │
│  └──────────────────┘  │   - tokens (enc) │              │
│                        │   - webhook_exp  │              │
│  ┌──────────────────┐  └──────────────────┘              │
│  │ tenant_org456    │                                     │
│  │ ──────────────── │  ┌──────────────────┐              │
│  │ • EmailAccount   │  │ tenant_org789    │              │
│  │   - account_id   │  │ ──────────────── │              │
│  │   - email_addr   │  │ • EmailAccount   │              │
│  │   - tokens (enc) │  │   - account_id   │              │
│  │   - webhook_exp  │  │   - email_addr   │              │
│  └──────────────────┘  │   - tokens (enc) │              │
│                        │   - webhook_exp  │              │
│                        └──────────────────┘              │
│                                                            │
│  The refresh job queries ALL namespaces to find all       │
│  active accounts that need watch renewal.                 │
└───────────────────────────────────────────────────────────┘
```

## Watch Expiration Timeline

```
Day 0                Day 5               Day 7
│                    │                   │
│ Watch created      │ Refresh triggered │ Old watch expires
│ Expiration: Day 7  │ (48h before exp)  │ New watch active
│                    │                   │
▼                    ▼                   ▼
┌────────────────────┬───────────────────┬─────────────────┐
│                    │                   │                 │
│  Watch active      │  Watch refreshed  │  New 7-day      │
│  historyId: 12345  │  New expiration:  │  cycle begins   │
│                    │  Day 12           │                 │
│                    │                   │                 │
└────────────────────┴───────────────────┴─────────────────┘
                                         │
                                         │
                     Day 10              ▼              Day 12
                     │                                  │
                     │ Refresh triggered again          │ Old watch expires
                     │ (48h before new exp)             │ Newer watch active
                     │                                  │
                     ▼                                  ▼
           ┌─────────────────────────────────────────────┐
           │                                             │
           │  Watch refreshed again                      │
           │  New expiration: Day 17                     │
           │                                             │
           └─────────────────────────────────────────────┘

This pattern continues indefinitely, ensuring watches never expire.
```

## Security Layers

```
┌──────────────────────────────────────────────────────────┐
│                  Security Architecture                    │
│                                                           │
│  Layer 1: Network                                         │
│  ────────────────                                         │
│  • HTTPS only                                             │
│  • Cloud Run/GKE ingress                                  │
│  • Optional: VPC Service Controls                         │
│                                                           │
│  Layer 2: Authentication                                  │
│  ──────────────────────                                   │
│  • OIDC token from Cloud Scheduler                        │
│  • Token signature validation                             │
│  • Service account verification                           │
│                                                           │
│  Layer 3: Authorization                                   │
│  ─────────────────────                                    │
│  • IAM roles (run.invoker)                               │
│  • Service account has minimal permissions                │
│  • No direct access to data stores                        │
│                                                           │
│  Layer 4: Data Protection                                 │
│  ────────────────────────                                 │
│  • OAuth tokens encrypted at rest (KMS)                   │
│  • Tokens decrypted only when needed                      │
│  • Namespace isolation for tenant data                    │
│                                                           │
│  Layer 5: Audit & Monitoring                              │
│  ───────────────────────                                  │
│  • All requests logged                                    │
│  • Cloud Audit Logs enabled                               │
│  • Alerts on failures                                     │
│                                                           │
└──────────────────────────────────────────────────────────┘
```

## Deployment

### Using gcloud CLI

```
backend/infrastructure/gcloud/
└── setup-cloud-scheduler.sh    # All-in-one script

Commands:
$ cd backend/infrastructure/gcloud
$ chmod +x setup-cloud-scheduler.sh
$ ./setup-cloud-scheduler.sh
```

> **Note:** Terraform configuration can be added later for production deployments with IaC requirements.

## Error Handling & Retries

```
┌─────────────────────────────────────────────────────────┐
│              Cloud Scheduler Retry Logic                 │
│                                                          │
│  Attempt 1                                               │
│  ─────────                                               │
│  Request sent → Failed (500 error)                       │
│                                                          │
│  Wait 5 seconds (min backoff)                            │
│  ▼                                                       │
│  Attempt 2                                               │
│  ─────────                                               │
│  Request sent → Failed (timeout)                         │
│                                                          │
│  Wait 10 seconds (exponential backoff)                   │
│  ▼                                                       │
│  Attempt 3                                               │
│  ─────────                                               │
│  Request sent → Success! ✓                               │
│                                                          │
│  If all 3 attempts fail:                                 │
│  • Job marked as failed                                  │
│  • Error logged to Cloud Logging                         │
│  • Will retry at next scheduled time                     │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

## Monitoring Dashboard

```
┌──────────────────────────────────────────────────────────┐
│            Recommended Monitoring Metrics                 │
│                                                           │
│  ┌─────────────────────────────────────────────────┐     │
│  │  Job Execution Rate                             │     │
│  │  ────────────────────                           │     │
│  │  📊 Line chart showing daily executions         │     │
│  │  Target: 1 execution per day                    │     │
│  │  Alert: < 1 execution in 36 hours               │     │
│  └─────────────────────────────────────────────────┘     │
│                                                           │
│  ┌─────────────────────────────────────────────────┐     │
│  │  Refresh Success Rate                           │     │
│  │  ───────────────────                            │     │
│  │  📊 Percentage of successfully refreshed accts  │     │
│  │  Target: > 95%                                  │     │
│  │  Alert: < 90%                                   │     │
│  └─────────────────────────────────────────────────┘     │
│                                                           │
│  ┌─────────────────────────────────────────────────┐     │
│  │  Accounts Needing Refresh                       │     │
│  │  ────────────────────────                       │     │
│  │  📊 Number of accounts refreshed per run        │     │
│  │  Helps predict scale needs                      │     │
│  └─────────────────────────────────────────────────┘     │
│                                                           │
│  ┌─────────────────────────────────────────────────┐     │
│  │  Job Duration                                   │     │
│  │  ────────────                                   │     │
│  │  📊 Time taken to complete                      │     │
│  │  Target: < 2 minutes                            │     │
│  │  Alert: > 4 minutes (approaching timeout)       │     │
│  └─────────────────────────────────────────────────┘     │
│                                                           │
└──────────────────────────────────────────────────────────┘
```

## Cost Breakdown

```
┌────────────────────────────────────────────────────────┐
│                   Monthly Cost Estimate                 │
│                                                         │
│  Cloud Scheduler:                                       │
│  • Jobs (1): $0.00 (within free tier of 3 jobs)        │
│  • Executions (30): $0.00 (within 250k free tier)      │
│                                                         │
│  Cloud Run (if applicable):                             │
│  • Invocations (30): $0.00 (within 2M free tier)       │
│  • Compute time: ~$0.001                                │
│                                                         │
│  Datastore:                                             │
│  • Reads (30 queries): ~$0.001                          │
│  • Writes (5-10 updates): ~$0.001                       │
│                                                         │
│  KMS:                                                   │
│  • Decryption ops (5-10): $0.00 (within free tier)     │
│                                                         │
│  Gmail API:                                             │
│  • Watch refresh calls (5-10): $0.00 (free)            │
│                                                         │
│  ─────────────────────────────────────────────────     │
│  TOTAL: ~$0.01/month                                    │
│                                                         │
└────────────────────────────────────────────────────────┘
```

## Scalability

The system scales automatically with:

- **10 accounts**: Job completes in ~30 seconds
- **100 accounts**: Job completes in ~2 minutes
- **1,000 accounts**: Job completes in ~4 minutes
- **10,000+ accounts**: Consider:
  - Increasing job timeout
  - Splitting into multiple jobs by region/tenant
  - Running refresh more frequently (every 12 hours)

No code changes needed for scaling up to thousands of accounts.
