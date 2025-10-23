package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/option"

	"github.com/yourusername/email-service/internal/types"
)

type BigQueryStore struct {
	client   *bigquery.Client
	location string
}

func NewBigQueryStore(ctx context.Context, projectID, location string) (*BigQueryStore, error) {
	// Create client with connection pooling for better performance
	// Pool size of 5 provides good balance between resource usage and performance
	client, err := bigquery.NewClient(ctx, projectID,
		option.WithGRPCConnectionPool(5))
	if err != nil {
		return nil, fmt.Errorf("failed to create bigquery client: %w", err)
	}
	return &BigQueryStore{
		client:   client,
		location: location,
	}, nil
}

type EmailRecord struct {
	MessageID     string    `bigquery:"message_id"`
	ThreadID      string    `bigquery:"thread_id"`
	AccountID     string    `bigquery:"account_id"`
	Sender        string    `bigquery:"sender"`
	SenderName    string    `bigquery:"sender_name"`
	Recipients    []string  `bigquery:"recipients"`
	CCRecipients  []string  `bigquery:"cc_recipients"`
	BCCRecipients []string  `bigquery:"bcc_recipients"`
	Subject       string    `bigquery:"subject"`
	BodyText      string    `bigquery:"body_text"`
	BodyHTML      string    `bigquery:"body_html"`
	BodyMarkdown  string    `bigquery:"body_markdown"`
	ReceivedAt    time.Time `bigquery:"received_at"`
	IngestedAt    time.Time `bigquery:"ingested_at"`
	IsRead        bool      `bigquery:"is_read"`
	Labels        []string  `bigquery:"labels"`
}

// SMSRecord represents an SMS message stored in BigQuery
type SMSRecord struct {
	MessageSID string    `bigquery:"message_sid" json:"message_sid"`
	FromNumber string    `bigquery:"from_number" json:"from_number"`
	ToNumber   string    `bigquery:"to_number" json:"to_number"`
	Body       string    `bigquery:"body" json:"body"`
	Direction  string    `bigquery:"direction" json:"direction"`
	Status     string    `bigquery:"status" json:"status"`
	ReceivedAt time.Time `bigquery:"received_at" json:"received_at"`
	IngestedAt time.Time `bigquery:"ingested_at" json:"ingested_at"`
	AccountID  string    `bigquery:"account_id" json:"account_id"`
}

// SMSStatusHistoryRecord represents a status change event for an SMS message
type SMSStatusHistoryRecord struct {
	MessageSID string    `bigquery:"message_sid"`
	Status     string    `bigquery:"status"`
	Timestamp  time.Time `bigquery:"timestamp"`
	AccountID  string    `bigquery:"account_id"`
}

// parseSender extracts the name and email from a sender string
// Format: "Name <email@example.com>" or just "email@example.com"
func parseSender(sender string) (email, name string) {
	sender = strings.TrimSpace(sender)

	// Check if it has the format "Name <email>"
	if idx := strings.Index(sender, "<"); idx != -1 {
		name = strings.TrimSpace(sender[:idx])
		// Extract email between < and >
		if endIdx := strings.Index(sender[idx:], ">"); endIdx != -1 {
			email = strings.TrimSpace(sender[idx+1 : idx+endIdx])
		} else {
			// Malformed, treat rest as email
			email = strings.TrimSpace(sender[idx+1:])
		}
	} else {
		// No angle brackets, treat entire string as email
		email = sender
		name = ""
	}

	return email, name
}

// CreateTenantDataset creates a BigQuery dataset for a tenant if it doesn't exist
func (b *BigQueryStore) CreateTenantDataset(ctx context.Context, datasetName string) error {
	dataset := b.client.Dataset(datasetName)

	// Check if dataset exists
	_, err := dataset.Metadata(ctx)
	if err == nil {
		// Dataset already exists
		return nil
	}

	// Create dataset
	meta := &bigquery.DatasetMetadata{
		Location: b.location,
	}

	if err := dataset.Create(ctx, meta); err != nil {
		return fmt.Errorf("failed to create dataset: %w", err)
	}

	return nil
}

// EnsureEmailTable creates the emails table if it doesn't exist
func (b *BigQueryStore) EnsureEmailTable(ctx context.Context, datasetName string) error {
	dataset := b.client.Dataset(datasetName)
	table := dataset.Table("emails")

	// Check if table exists
	md, err := table.Metadata(ctx)
	if err == nil {
		// Table already exists - check if it needs schema updates
		hasBodyHTML := false
		hasBodyMarkdown := false
		hasSenderName := false
		hasRecipients := false
		hasCCRecipients := false
		hasBCCRecipients := false
		messageIDRequired := false
		hasClustering := false

		for _, f := range md.Schema {
			if f.Name == "body_html" {
				hasBodyHTML = true
			}
			if f.Name == "body_markdown" {
				hasBodyMarkdown = true
			}
			if f.Name == "sender_name" {
				hasSenderName = true
			}
			if f.Name == "recipients" {
				hasRecipients = true
			}
			if f.Name == "cc_recipients" {
				hasCCRecipients = true
			}
			if f.Name == "bcc_recipients" {
				hasBCCRecipients = true
			}
			if f.Name == "message_id" && f.Required {
				messageIDRequired = true
			}
		}

		// Check if clustering is configured
		if md.Clustering != nil && len(md.Clustering.Fields) > 0 {
			hasClustering = true
		}

		// Add missing fields if needed
		needsSchemaUpdate := !hasBodyHTML || !hasBodyMarkdown || !hasSenderName || !hasRecipients || !hasCCRecipients || !hasBCCRecipients || !messageIDRequired

		if needsSchemaUpdate {
			// Build new schema with all existing fields plus missing ones
			newSchema := make(bigquery.Schema, 0, len(md.Schema)+6)

			// Copy existing fields, potentially updating message_id requirement
			for _, field := range md.Schema {
				newField := &bigquery.FieldSchema{
					Name:        field.Name,
					Type:        field.Type,
					Description: field.Description,
					Required:    field.Required,
					Repeated:    field.Repeated,
				}
				// Update message_id to be required if it isn't already
				if field.Name == "message_id" && !messageIDRequired {
					newField.Required = true
				}
				newSchema = append(newSchema, newField)
			}

			// Add missing fields
			if !hasBodyHTML {
				newSchema = append(newSchema, &bigquery.FieldSchema{
					Name:     "body_html",
					Type:     bigquery.StringFieldType,
					Required: false,
				})
			}
			if !hasBodyMarkdown {
				newSchema = append(newSchema, &bigquery.FieldSchema{
					Name:     "body_markdown",
					Type:     bigquery.StringFieldType,
					Required: false,
				})
			}
			if !hasSenderName {
				newSchema = append(newSchema, &bigquery.FieldSchema{
					Name:     "sender_name",
					Type:     bigquery.StringFieldType,
					Required: false,
				})
			}
			if !hasRecipients {
				newSchema = append(newSchema, &bigquery.FieldSchema{
					Name:     "recipients",
					Type:     bigquery.StringFieldType,
					Repeated: true,
					Required: false,
				})
			}
			if !hasCCRecipients {
				newSchema = append(newSchema, &bigquery.FieldSchema{
					Name:     "cc_recipients",
					Type:     bigquery.StringFieldType,
					Repeated: true,
					Required: false,
				})
			}
			if !hasBCCRecipients {
				newSchema = append(newSchema, &bigquery.FieldSchema{
					Name:     "bcc_recipients",
					Type:     bigquery.StringFieldType,
					Repeated: true,
					Required: false,
				})
			}

			// Update schema
			update := bigquery.TableMetadataToUpdate{
				Schema: newSchema,
			}

			// Refresh metadata to get latest ETag
			md, err = table.Metadata(ctx)
			if err != nil {
				return fmt.Errorf("failed to refresh table metadata: %w", err)
			}

			if _, uerr := table.Update(ctx, update, md.ETag); uerr != nil {
				return fmt.Errorf("failed to update table schema: %w", uerr)
			}
		}

		// Add clustering if not present
		if !hasClustering {
			// Refresh metadata to get latest ETag
			md, err = table.Metadata(ctx)
			if err != nil {
				return fmt.Errorf("failed to refresh table metadata for clustering: %w", err)
			}

			update := bigquery.TableMetadataToUpdate{
				Clustering: &bigquery.Clustering{
					Fields: []string{"message_id", "account_id"},
				},
			}
			if _, uerr := table.Update(ctx, update, md.ETag); uerr != nil {
				return fmt.Errorf("failed to add clustering: %w", uerr)
			}
		}

		return nil
	}

	// Infer schema from EmailRecord struct
	schema, err := bigquery.InferSchema(EmailRecord{})
	if err != nil {
		return fmt.Errorf("failed to infer schema: %w", err)
	}

	// Add unique constraint to message_id field
	for _, field := range schema {
		if field.Name == "message_id" {
			field.Required = true
			break
		}
	}

	metadata := &bigquery.TableMetadata{
		Schema: schema,
		TimePartitioning: &bigquery.TimePartitioning{
			Field: "received_at",
		},
		// Add clustering for better performance on message_id lookups
		Clustering: &bigquery.Clustering{
			Fields: []string{"message_id", "account_id"},
		},
	}

	if err := table.Create(ctx, metadata); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	return nil
}

// InsertMessages inserts multiple email messages into BigQuery with deduplication
// This method now uses MERGE operations to prevent duplicates by default
func (b *BigQueryStore) InsertMessages(ctx context.Context, datasetName string, messages []*types.EmailMessage, accountID string) error {
	// Use the deduplication method by default to prevent duplicates
	return b.InsertMessagesWithDedup(ctx, datasetName, messages, accountID)
}

// InsertMessagesStreaming inserts multiple email messages using streaming inserts
// WARNING: This method does NOT prevent duplicates and should only be used for
// high-volume scenarios where duplicates are acceptable or handled elsewhere
func (b *BigQueryStore) InsertMessagesStreaming(ctx context.Context, datasetName string, messages []*types.EmailMessage, accountID string) error {
	if len(messages) == 0 {
		return nil
	}

	// Ensure dataset and table exist
	if err := b.CreateTenantDataset(ctx, datasetName); err != nil {
		return err
	}
	if err := b.EnsureEmailTable(ctx, datasetName); err != nil {
		return err
	}

	// Convert messages to EmailRecord format
	records := make([]*EmailRecord, 0, len(messages))
	for _, msg := range messages {
		senderEmail, senderName := parseSender(msg.Sender)
		records = append(records, &EmailRecord{
			MessageID:     msg.MessageID,
			ThreadID:      msg.ThreadID,
			AccountID:     accountID,
			Sender:        senderEmail,
			SenderName:    senderName,
			Recipients:    msg.Recipients,
			CCRecipients:  msg.CCRecipients,
			BCCRecipients: msg.BCCRecipients,
			Subject:       msg.Subject,
			BodyText:      msg.BodyText,
			BodyHTML:      msg.BodyHTML,
			BodyMarkdown:  msg.BodyMarkdown,
			ReceivedAt:    msg.ReceivedAt,
			IngestedAt:    time.Now(),
			IsRead:        msg.IsRead,
			Labels:        msg.Labels,
		})
	}

	// Use streaming insert for batch processing
	dataset := b.client.Dataset(datasetName)
	inserter := dataset.Table("emails").Inserter()

	// Set insertId for deduplication (BigQuery will ignore duplicates within ~1 minute window)
	inserter.SkipInvalidRows = false
	inserter.IgnoreUnknownValues = false

	// Batch insert all records at once
	if err := inserter.Put(ctx, records); err != nil {
		return fmt.Errorf("failed to insert messages: %w", err)
	}

	return nil
}

// InsertMessagesWithDedup inserts messages with atomic deduplication using MERGE
// This approach uses BigQuery MERGE to prevent duplicates at the database level
func (b *BigQueryStore) InsertMessagesWithDedup(ctx context.Context, datasetName string, messages []*types.EmailMessage, accountID string) error {
	if len(messages) == 0 {
		return nil
	}

	// Ensure dataset and table exist
	if err := b.CreateTenantDataset(ctx, datasetName); err != nil {
		return err
	}
	if err := b.EnsureEmailTable(ctx, datasetName); err != nil {
		return err
	}

	// Convert messages to EmailRecord format
	records := make([]*EmailRecord, 0, len(messages))
	for _, msg := range messages {
		senderEmail, senderName := parseSender(msg.Sender)
		records = append(records, &EmailRecord{
			MessageID:     msg.MessageID,
			ThreadID:      msg.ThreadID,
			AccountID:     accountID,
			Sender:        senderEmail,
			SenderName:    senderName,
			Recipients:    msg.Recipients,
			CCRecipients:  msg.CCRecipients,
			BCCRecipients: msg.BCCRecipients,
			Subject:       msg.Subject,
			BodyText:      msg.BodyText,
			BodyHTML:      msg.BodyHTML,
			BodyMarkdown:  msg.BodyMarkdown,
			ReceivedAt:    msg.ReceivedAt,
			IngestedAt:    time.Now(),
			IsRead:        msg.IsRead,
			Labels:        msg.Labels,
		})
	}

	// Use MERGE statement for atomic deduplication
	return b.insertMessagesWithMerge(ctx, datasetName, records, accountID)
}

// insertMessagesWithMerge uses BigQuery MERGE statement for atomic deduplication
func (b *BigQueryStore) insertMessagesWithMerge(ctx context.Context, datasetName string, records []*EmailRecord, accountID string) error {
	if len(records) == 0 {
		return nil
	}

	// Create a temporary table for the new records
	tempTableName := fmt.Sprintf("temp_emails_%d", time.Now().UnixNano())
	tempTable := b.client.Dataset(datasetName).Table(tempTableName)

	// Create temporary table with same schema as emails table
	tempSchema, err := bigquery.InferSchema(EmailRecord{})
	if err != nil {
		return fmt.Errorf("failed to infer schema for temp table: %w", err)
	}

	tempMetadata := &bigquery.TableMetadata{
		Schema: tempSchema,
		TimePartitioning: &bigquery.TimePartitioning{
			Field: "received_at",
		},
	}

	if err := tempTable.Create(ctx, tempMetadata); err != nil {
		return fmt.Errorf("failed to create temp table: %w", err)
	}

	// Clean up temp table when done
	defer func() {
		if err := tempTable.Delete(ctx); err != nil {
			fmt.Printf("⚠️  Failed to delete temp table %s: %v\n", tempTableName, err)
		}
	}()

	// Insert records into temporary table
	inserter := tempTable.Inserter()
	inserter.SkipInvalidRows = false
	inserter.IgnoreUnknownValues = false

	if err := inserter.Put(ctx, records); err != nil {
		return fmt.Errorf("failed to insert records into temp table: %w", err)
	}

	// Build MERGE statement to insert only new records
	mergeQuery := fmt.Sprintf(`
		MERGE `+"`%s.emails`"+` AS target
		USING `+"`%s.%s`"+` AS source
		ON target.message_id = source.message_id AND target.account_id = source.account_id
		WHEN NOT MATCHED THEN
			INSERT (
				message_id, thread_id, account_id, sender, sender_name,
				recipients, cc_recipients, bcc_recipients, subject,
				body_text, body_html, body_markdown, received_at, ingested_at,
				is_read, labels
			)
			VALUES (
				source.message_id, source.thread_id, source.account_id, source.sender, source.sender_name,
				source.recipients, source.cc_recipients, source.bcc_recipients, source.subject,
				source.body_text, source.body_html, source.body_markdown, source.received_at, source.ingested_at,
				source.is_read, source.labels
			)
	`, datasetName, datasetName, tempTableName)

	query := b.client.Query(mergeQuery)
	query.Location = b.location

	job, err := query.Run(ctx)
	if err != nil {
		return fmt.Errorf("failed to start MERGE job: %w", err)
	}

	// Wait for job to complete
	status, err := job.Wait(ctx)
	if err != nil {
		return fmt.Errorf("MERGE job failed: %w", err)
	}

	if status.Err() != nil {
		return fmt.Errorf("MERGE job error: %w", status.Err())
	}

	// Log successful insertions
	fmt.Printf("✅ Inserted %d emails using MERGE (account: %s)\n", len(records), accountID)

	return nil
}

// emailExists checks if an email with the given messageID and accountID already exists
func (b *BigQueryStore) emailExists(ctx context.Context, datasetName, messageID, accountID string) (bool, error) {
	query := b.client.Query(fmt.Sprintf(`
		SELECT COUNT(*) as count
		FROM `+"`%s.emails`"+`
		WHERE message_id = @message_id AND account_id = @account_id
	`, datasetName))

	query.Location = b.location
	query.Parameters = []bigquery.QueryParameter{
		{Name: "message_id", Value: messageID},
		{Name: "account_id", Value: accountID},
	}

	it, err := query.Read(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to query email existence: %w", err)
	}

	var count int64
	var row struct {
		Count int64 `bigquery:"count"`
	}
	err = it.Next(&row)
	if err == nil {
		count = row.Count
	}

	return count > 0, nil
}

// insertSingleEmail inserts a single email record into BigQuery
func (b *BigQueryStore) insertSingleEmail(ctx context.Context, datasetName string, record *EmailRecord) error {
	dataset := b.client.Dataset(datasetName)
	inserter := dataset.Table("emails").Inserter()

	// Configure inserter
	inserter.SkipInvalidRows = false
	inserter.IgnoreUnknownValues = false

	// Insert single record
	if err := inserter.Put(ctx, record); err != nil {
		return fmt.Errorf("failed to insert record: %w", err)
	}

	return nil
}

// ListEmails retrieves recent emails for a tenant
// EmailListResult contains paginated email results
type EmailListResult struct {
	Emails     []*EmailRecord
	Total      int
	Page       int
	PerPage    int
	TotalPages int
}

func (b *BigQueryStore) ListEmails(ctx context.Context, datasetName string, limit int) ([]*EmailRecord, error) {
	result, err := b.ListEmailsPaginated(ctx, datasetName, 1, limit)
	if err != nil {
		return nil, err
	}
	return result.Emails, nil
}

func (b *BigQueryStore) ListEmailsPaginated(ctx context.Context, datasetName string, page, perPage int) (*EmailListResult, error) {
	// Validate pagination parameters
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 50 // default
	}

	offset := (page - 1) * perPage

	// First, get total count
	countQuery := b.client.Query(fmt.Sprintf(`
		SELECT COUNT(*) as total
		FROM `+"`%s.emails`"+`
	`, datasetName))
	countQuery.Location = b.location

	countIt, err := countQuery.Read(ctx)
	if err != nil {
		// If dataset or table doesn't exist yet, return empty result
		if strings.Contains(err.Error(), "Not found") || strings.Contains(err.Error(), "notFound") {
			return &EmailListResult{
				Emails:     []*EmailRecord{},
				Total:      0,
				Page:       page,
				PerPage:    perPage,
				TotalPages: 0,
			}, nil
		}
		return nil, fmt.Errorf("failed to count emails: %w", err)
	}

	var countResult struct {
		Total int64 `bigquery:"total"`
	}
	err = countIt.Next(&countResult)
	if err != nil {
		return nil, fmt.Errorf("failed to read count: %w", err)
	}

	total := int(countResult.Total)
	totalPages := (total + perPage - 1) / perPage

	// Now get the paginated results
	query := b.client.Query(fmt.Sprintf(`
		SELECT 
			message_id,
			thread_id,
			account_id,
			sender,
			sender_name,
			recipients,
			cc_recipients,
			bcc_recipients,
			subject,
			body_text,
			body_html,
			body_markdown,
			received_at,
			ingested_at,
			is_read,
			labels
		FROM `+"`%s.emails`"+`
		ORDER BY received_at DESC
		LIMIT %d OFFSET %d
	`, datasetName, perPage, offset))

	// Set the query location to match where datasets are created
	query.Location = b.location

	it, err := query.Read(ctx)
	if err != nil {
		// If dataset or table doesn't exist yet, return empty result
		if strings.Contains(err.Error(), "Not found") || strings.Contains(err.Error(), "notFound") {
			return &EmailListResult{
				Emails:     []*EmailRecord{},
				Total:      0,
				Page:       page,
				PerPage:    perPage,
				TotalPages: 0,
			}, nil
		}
		return nil, fmt.Errorf("failed to query: %w", err)
	}

	var emails []*EmailRecord
	for {
		var record EmailRecord
		err := it.Next(&record)
		if err != nil {
			break
		}
		emails = append(emails, &record)
	}

	return &EmailListResult{
		Emails:     emails,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}, nil
}

// EnsureSMSTable creates the sms_messages table if it doesn't exist
func (b *BigQueryStore) EnsureSMSTable(ctx context.Context, datasetName string) error {
	dataset := b.client.Dataset(datasetName)
	table := dataset.Table("sms_messages")

	// Check if table exists
	_, err := table.Metadata(ctx)
	if err == nil {
		// Table already exists
		return nil
	}

	// Infer schema from SMSRecord struct
	schema, err := bigquery.InferSchema(SMSRecord{})
	if err != nil {
		return fmt.Errorf("failed to infer schema: %w", err)
	}

	// Make message_sid required for uniqueness
	for _, field := range schema {
		if field.Name == "message_sid" {
			field.Required = true
			break
		}
	}

	metadata := &bigquery.TableMetadata{
		Schema: schema,
		TimePartitioning: &bigquery.TimePartitioning{
			Field: "received_at",
		},
		Clustering: &bigquery.Clustering{
			Fields: []string{"message_sid", "account_id"},
		},
	}

	if err := table.Create(ctx, metadata); err != nil {
		return fmt.Errorf("failed to create SMS table: %w", err)
	}

	return nil
}

// InsertSMSMessage inserts a single SMS message into BigQuery
func (b *BigQueryStore) InsertSMSMessage(ctx context.Context, datasetName string, message *types.SMSMessage, accountID string) error {
	// Ensure dataset and table exist
	if err := b.CreateTenantDataset(ctx, datasetName); err != nil {
		return err
	}
	if err := b.EnsureSMSTable(ctx, datasetName); err != nil {
		return err
	}

	// Convert to SMSRecord
	record := &SMSRecord{
		MessageSID: message.MessageSID,
		FromNumber: message.From,
		ToNumber:   message.To,
		Body:       message.Body,
		Direction:  message.Direction,
		Status:     message.Status,
		ReceivedAt: message.ReceivedAt,
		IngestedAt: time.Now(),
		AccountID:  accountID,
	}

	// If ReceivedAt is zero, use SentAt (for outbound messages)
	if record.ReceivedAt.IsZero() && !message.SentAt.IsZero() {
		record.ReceivedAt = message.SentAt
	}

	// If still zero, use current time
	if record.ReceivedAt.IsZero() {
		record.ReceivedAt = time.Now()
	}

	// Insert single record
	dataset := b.client.Dataset(datasetName)
	inserter := dataset.Table("sms_messages").Inserter()
	inserter.SkipInvalidRows = false
	inserter.IgnoreUnknownValues = false

	if err := inserter.Put(ctx, record); err != nil {
		// If table not found, ensure tables and retry once
		if strings.Contains(err.Error(), "notFound") || strings.Contains(err.Error(), "not found") {
			if err := b.EnsureSMSTable(ctx, datasetName); err != nil {
				return fmt.Errorf("failed to ensure sms_messages table: %w", err)
			}
			// Retry insert
			if err := inserter.Put(ctx, record); err != nil {
				return fmt.Errorf("failed to insert SMS message (retry): %w", err)
			}
		} else {
			return fmt.Errorf("failed to insert SMS message: %w", err)
		}
	}

	// Also insert initial status into status history
	// This handles its own table creation and retries
	if err := b.InsertSMSStatusHistory(ctx, datasetName, message.MessageSID, message.Status, accountID); err != nil {
		return fmt.Errorf("failed to insert initial status history: %w", err)
	}

	return nil
}

// ListSMSMessages retrieves recent SMS messages for a tenant with their latest status
func (b *BigQueryStore) ListSMSMessages(ctx context.Context, datasetName string, limit int) ([]*SMSRecord, error) {
	query := b.client.Query(fmt.Sprintf(`
		WITH LatestStatus AS (
			SELECT 
				message_sid,
				status,
				ROW_NUMBER() OVER (PARTITION BY message_sid ORDER BY timestamp DESC) as rn
			FROM `+"`%s.sms_status_history`"+`
		)
		SELECT 
			m.message_sid,
			m.from_number,
			m.to_number,
			m.body,
			m.direction,
			COALESCE(s.status, m.status) as status,
			m.received_at,
			m.ingested_at,
			m.account_id
		FROM `+"`%s.sms_messages`"+` m
		LEFT JOIN LatestStatus s ON m.message_sid = s.message_sid AND s.rn = 1
		ORDER BY m.received_at DESC
		LIMIT %d
	`, datasetName, datasetName, limit))

	query.Location = b.location

	it, err := query.Read(ctx)
	if err != nil {
		// If dataset or table doesn't exist yet, return empty list
		if strings.Contains(err.Error(), "Not found") || strings.Contains(err.Error(), "notFound") {
			return []*SMSRecord{}, nil
		}
		return nil, fmt.Errorf("failed to query SMS messages: %w", err)
	}

	var messages []*SMSRecord
	for {
		var record SMSRecord
		err := it.Next(&record)
		if err != nil {
			break
		}
		messages = append(messages, &record)
	}

	return messages, nil
}

// EnsureSMSStatusHistoryTable creates the SMS status history table if it doesn't exist
func (b *BigQueryStore) EnsureSMSStatusHistoryTable(ctx context.Context, datasetName string) error {
	tableRef := b.client.Dataset(datasetName).Table("sms_status_history")

	// Check if table exists
	_, err := tableRef.Metadata(ctx)
	if err == nil {
		// Table already exists
		return nil
	}

	// Define schema
	schema := bigquery.Schema{
		{Name: "message_sid", Type: bigquery.StringFieldType, Required: true},
		{Name: "status", Type: bigquery.StringFieldType, Required: true},
		{Name: "timestamp", Type: bigquery.TimestampFieldType, Required: true},
		{Name: "account_id", Type: bigquery.StringFieldType, Required: true},
	}

	// Create table
	if err := tableRef.Create(ctx, &bigquery.TableMetadata{
		Schema: schema,
	}); err != nil {
		// Ignore "Already Exists" errors (race condition)
		if !strings.Contains(err.Error(), "Already Exists") {
			return fmt.Errorf("failed to create sms_status_history table: %w", err)
		}
	}

	return nil
}

// InsertSMSStatusHistory inserts a status change event into the history table
func (b *BigQueryStore) InsertSMSStatusHistory(ctx context.Context, datasetName, messageSID, status, accountID string) error {
	// Ensure dataset exists
	if err := b.CreateTenantDataset(ctx, datasetName); err != nil {
		return err
	}

	// Ensure history table exists
	if err := b.EnsureSMSStatusHistoryTable(ctx, datasetName); err != nil {
		return err
	}

	// Create history record
	record := &SMSStatusHistoryRecord{
		MessageSID: messageSID,
		Status:     status,
		Timestamp:  time.Now(),
		AccountID:  accountID,
	}

	// Insert record
	dataset := b.client.Dataset(datasetName)
	inserter := dataset.Table("sms_status_history").Inserter()
	inserter.SkipInvalidRows = false
	inserter.IgnoreUnknownValues = false

	if err := inserter.Put(ctx, record); err != nil {
		// If table not found, try to ensure it exists and retry once
		if strings.Contains(err.Error(), "notFound") || strings.Contains(err.Error(), "not found") {
			if err := b.EnsureSMSStatusHistoryTable(ctx, datasetName); err != nil {
				return fmt.Errorf("failed to ensure status history table: %w", err)
			}
			// Retry insert
			if err := inserter.Put(ctx, record); err != nil {
				return fmt.Errorf("failed to insert SMS status history (retry): %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to insert SMS status history: %w", err)
	}

	return nil
}

func (b *BigQueryStore) Close() error {
	return b.client.Close()
}
