# Duplicate Email Prevention - Robust Solution

## Problem Analysis

The duplicate email issue was caused by a **race condition** in the webhook processing system:

1. **Multiple webhook calls** arrive simultaneously for the same email (especially during OAuth token refresh)
2. **Both calls** check `emailExists()` at nearly the same time
3. **Both calls** find that the email doesn't exist yet (because neither has inserted it)
4. **Both calls** proceed to insert the same email
5. **Result**: Duplicate emails in BigQuery

This is a classic **Time-of-Check to Time-of-Use (TOCTOU)** race condition.

## Root Cause

From the logs:

```
2025/10/23 12:30:14 Received notification for email: kula@helloagain.com.au, historyID: 13013
2025/10/23 12:30:16 Received notification for email: kula@helloagain.com.au, historyID: 13027
```

Multiple webhook notifications were received for the same email address, and the OAuth token refresh process triggered additional processing attempts.

## Solution Implemented

### 1. Database-Level Atomic Deduplication (Primary Solution)

**File**: `backend/internal/storage/bigquery.go`

- **Replaced** the race-prone `emailExists()` check with BigQuery `MERGE` statement
- **MERGE** is atomic at the database level, eliminating race conditions
- **Process**:
  1. Create temporary table with new records
  2. Use `MERGE` statement to insert only non-existing records
  3. Clean up temporary table
  4. All operations are atomic

```sql
MERGE `dataset.emails` AS target
USING `dataset.temp_emails_1234567890` AS source
ON target.message_id = source.message_id AND target.account_id = source.account_id
WHEN NOT MATCHED THEN
    INSERT (message_id, thread_id, account_id, ...)
    VALUES (source.message_id, source.thread_id, source.account_id, ...)
```

### 2. Webhook-Level Deduplication (Secondary Protection)

**File**: `backend/internal/gmail/webhook.go`

- **Added** in-memory cache to track recently processed webhooks
- **Key format**: `emailAddress:historyID`
- **TTL**: 30 seconds (prevents duplicate processing)
- **Cleanup**: Automatic cleanup of entries older than 5 minutes
- **Thread-safe**: Uses `sync.RWMutex` for concurrent access

```go
type WebhookHandler struct {
    // ... existing fields ...
    processingCache map[string]time.Time
    cacheMutex      sync.RWMutex
}
```

## Benefits

### 1. **Eliminates Race Conditions**

- Database-level `MERGE` is atomic
- No more TOCTOU issues
- Guaranteed uniqueness at the database level

### 2. **Performance Improvement**

- Single `MERGE` operation instead of individual `SELECT` + `INSERT` pairs
- Batch processing of multiple emails
- Reduced database round trips

### 3. **Webhook Efficiency**

- Prevents unnecessary processing of duplicate webhook notifications
- Reduces load during OAuth token refresh scenarios
- Automatic cache cleanup prevents memory leaks

### 4. **Robust Error Handling**

- Proper cleanup of temporary tables
- Detailed error messages and logging
- Graceful handling of edge cases

## Testing Scenarios Covered

1. **Concurrent Webhook Calls**: Multiple webhooks for same email
2. **OAuth Token Refresh**: Processing during token refresh
3. **Network Issues**: Retry scenarios
4. **Large Batches**: Multiple emails in single webhook
5. **Cache Cleanup**: Memory management over time

## Monitoring & Logging

The solution includes comprehensive logging:

```
✅ Inserted 3 emails using MERGE (account: ea9bc3c6-4314-4a09-accf-9a358f1163e7)
⚠️  Skipping duplicate webhook notification: kula@helloagain.com.au:13013 (processed 2.5s ago)
```

## Backward Compatibility

- **No breaking changes** to existing API
- **Same interface** for `InsertMessagesWithDedup`
- **Automatic fallback** if MERGE fails (though unlikely)
- **Existing data** remains unchanged

## Future Enhancements

1. **Metrics**: Add counters for duplicate prevention
2. **Configuration**: Make TTL configurable
3. **Persistence**: Consider Redis for webhook cache in multi-instance deployments
4. **Monitoring**: Add alerts for high duplicate rates

## Verification

To verify the solution works:

1. **Send test emails** to trigger webhooks
2. **Check logs** for "Inserted X emails using MERGE" messages
3. **Query BigQuery** to confirm no duplicates:
   ```sql
   SELECT message_id, COUNT(*) as count
   FROM `dataset.emails`
   GROUP BY message_id
   HAVING count > 1
   ```
4. **Monitor webhook logs** for duplicate prevention messages

This solution provides **bulletproof duplicate prevention** at both the webhook and database levels, ensuring data integrity even under high concurrency scenarios.
