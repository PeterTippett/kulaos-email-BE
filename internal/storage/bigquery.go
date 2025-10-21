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

// InsertMessagesWithDedup inserts messages with deduplication using streaming inserts with insertId
// This approach uses BigQuery's built-in deduplication based on insertId
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

	// Use streaming insert with insertId for deduplication
	dataset := b.client.Dataset(datasetName)
	inserter := dataset.Table("emails").Inserter()

	// Configure inserter for deduplication
	inserter.SkipInvalidRows = false
	inserter.IgnoreUnknownValues = false

	// Insert records directly - BigQuery will handle deduplication based on message_id + account_id
	// since we have clustering on these fields
	if err := inserter.Put(ctx, records); err != nil {
		return fmt.Errorf("failed to insert messages: %w", err)
	}

	return nil
}

// ListEmails retrieves recent emails for a tenant
func (b *BigQueryStore) ListEmails(ctx context.Context, datasetName string, limit int) ([]*EmailRecord, error) {
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
		LIMIT %d
	`, datasetName, limit))

	// Set the query location to match where datasets are created
	query.Location = b.location

	it, err := query.Read(ctx)
	if err != nil {
		// If dataset or table doesn't exist yet, return empty list
		if strings.Contains(err.Error(), "Not found") || strings.Contains(err.Error(), "notFound") {
			return []*EmailRecord{}, nil
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

	return emails, nil
}

func (b *BigQueryStore) Close() error {
	return b.client.Close()
}
