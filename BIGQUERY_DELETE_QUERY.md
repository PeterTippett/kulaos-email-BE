# BigQuery Query to Remove Specific Message

## SQL Query

```sql
-- Delete a specific message by message_id
DELETE FROM `your-project.your-dataset.emails`
WHERE message_id = 'your-message-id-here'
```

## Example with Actual Values

```sql
-- Delete the duplicate email from your logs
DELETE FROM `your-project.org_3473ac166f007.emails`
WHERE message_id = '19a0eb013debe213'
```

## Safer Approach - Check First, Then Delete

```sql
-- Step 1: Check how many records exist for this message_id
SELECT
  message_id,
  account_id,
  subject,
  received_at,
  ingested_at,
  COUNT(*) as duplicate_count
FROM `your-project.org_3473ac166f007.emails`
WHERE message_id = '19a0eb013debe213'
GROUP BY message_id, account_id, subject, received_at, ingested_at;

-- Step 2: If you see duplicates, delete all instances
DELETE FROM `your-project.org_3473ac166f007.emails`
WHERE message_id = '19a0eb013debe213';
```

## Delete Only Duplicates (Keep One Instance)

```sql
-- Delete duplicates but keep the first instance (by ingested_at)
DELETE FROM `your-project.org_3473ac166f007.emails`
WHERE message_id = '19a0eb013debe213'
  AND ingested_at NOT IN (
    SELECT MIN(ingested_at)
    FROM `your-project.org_3473ac166f007.emails`
    WHERE message_id = '19a0eb013debe213'
  );
```

## Delete by Multiple Criteria

```sql
-- Delete specific message for specific account
DELETE FROM `your-project.org_3473ac166f007.emails`
WHERE message_id = '19a0eb013debe213'
  AND account_id = 'ea9bc3c6-4314-4a09-accf-9a358f1163e7';
```

## How to Run the Query

### Option 1: BigQuery Console

1. Go to [BigQuery Console](https://console.cloud.google.com/bigquery)
2. Select your project
3. Click "Compose new query"
4. Paste the SQL query
5. Replace placeholders with actual values
6. Click "Run"

### Option 2: Command Line (gcloud)

```bash
# Set your project
gcloud config set project your-project-id

# Run the query
bq query --use_legacy_sql=false '
DELETE FROM `your-project.org_3473ac166f007.emails`
WHERE message_id = "19a0eb013debe213"
'
```

### Option 3: Using bq CLI with Dry Run First

```bash
# First, dry run to see what would be affected
bq query --use_legacy_sql=false --dry_run '
DELETE FROM `your-project.org_3473ac166f007.emails`
WHERE message_id = "19a0eb013debe213"
'

# If dry run looks good, run the actual delete
bq query --use_legacy_sql=false '
DELETE FROM `your-project.org_3473ac166f007.emails`
WHERE message_id = "19a0eb013debe213"
'
```

## Important Notes

⚠️ **WARNING**: DELETE operations in BigQuery are permanent and cannot be undone!

1. **Always test first**: Use SELECT queries to verify the records you want to delete
2. **Use dry run**: Test with `--dry_run` flag first
3. **Backup if needed**: Consider exporting data before deletion
4. **Check permissions**: Ensure you have DELETE permissions on the table

## Verification Query

After deletion, verify the message is gone:

```sql
-- Check if the message still exists
SELECT COUNT(*) as remaining_count
FROM `your-project.org_3473ac166f007.emails`
WHERE message_id = '19a0eb013debe213';
```

## For Your Specific Case

Based on your logs, to remove the duplicate email `19a0eb013debe213`:

```sql
-- Check current state
SELECT
  message_id,
  account_id,
  subject,
  received_at,
  ingested_at,
  COUNT(*) as count
FROM `your-project.org_3473ac166f007.emails`
WHERE message_id = '19a0eb013debe213'
GROUP BY message_id, account_id, subject, received_at, ingested_at;

-- Delete all instances (if you want to remove completely)
DELETE FROM `your-project.org_3473ac166f007.emails`
WHERE message_id = '19a0eb013debe213';

-- OR delete duplicates but keep the first one
DELETE FROM `your-project.org_3473ac166f007.emails`
WHERE message_id = '19a0eb013debe213'
  AND ingested_at NOT IN (
    SELECT MIN(ingested_at)
    FROM `your-project.org_3473ac166f007.emails`
    WHERE message_id = '19a0eb013debe213'
  );
```
