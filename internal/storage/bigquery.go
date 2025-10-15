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
	MessageID    string    `bigquery:"message_id"`
	ThreadID     string    `bigquery:"thread_id"`
	AccountID    string    `bigquery:"account_id"`
	Sender       string    `bigquery:"sender"`
	Subject      string    `bigquery:"subject"`
	BodyText     string    `bigquery:"body_text"`
	BodyHTML     string    `bigquery:"body_html"`
	BodyMarkdown string    `bigquery:"body_markdown"`
	ReceivedAt   time.Time `bigquery:"received_at"`
	IngestedAt   time.Time `bigquery:"ingested_at"`
	IsRead       bool      `bigquery:"is_read"`
	Labels       []string  `bigquery:"labels"`
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
		messageIDRequired := false
		hasClustering := false

		for _, f := range md.Schema {
			if f.Name == "body_html" {
				hasBodyHTML = true
			}
			if f.Name == "body_markdown" {
				hasBodyMarkdown = true
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
		var newSchema bigquery.Schema
		var needsUpdate bool

		if !hasBodyHTML || !hasBodyMarkdown || !messageIDRequired {
			newSchema = make(bigquery.Schema, len(md.Schema))
			copy(newSchema, md.Schema)

			if !hasBodyHTML {
				newSchema = append(newSchema, &bigquery.FieldSchema{
					Name:     "body_html",
					Type:     bigquery.StringFieldType,
					Required: false,
				})
				needsUpdate = true
			}
			if !hasBodyMarkdown {
				newSchema = append(newSchema, &bigquery.FieldSchema{
					Name:     "body_markdown",
					Type:     bigquery.StringFieldType,
					Required: false,
				})
				needsUpdate = true
			}
			if !messageIDRequired {
				// Make message_id required for uniqueness
				for _, field := range newSchema {
					if field.Name == "message_id" {
						field.Required = true
						needsUpdate = true
						break
					}
				}
			}
		}

		// Update schema if needed
		if needsUpdate {
			update := bigquery.TableMetadataToUpdate{
				Schema: newSchema,
			}
			if _, uerr := table.Update(ctx, update, md.ETag); uerr != nil {
				return fmt.Errorf("failed to update table schema: %w", uerr)
			}
		}

		// Add clustering if not present
		if !hasClustering {
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

// InsertMessages inserts multiple email messages into BigQuery using streaming inserts
// Note: Streaming inserts do not support deduplication, so the application should
// handle duplicate prevention if necessary. BigQuery will deduplicate based on insertId
// within a limited time window.
func (b *BigQueryStore) InsertMessages(ctx context.Context, datasetName string, messages []*types.EmailMessage, accountID string) error {
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
		records = append(records, &EmailRecord{
			MessageID:    msg.MessageID,
			ThreadID:     msg.ThreadID,
			AccountID:    accountID,
			Sender:       msg.Sender,
			Subject:      msg.Subject,
			BodyText:     msg.BodyText,
			BodyHTML:     msg.BodyHTML,
			BodyMarkdown: msg.BodyMarkdown,
			ReceivedAt:   msg.ReceivedAt,
			IngestedAt:   time.Now(),
			IsRead:       msg.IsRead,
			Labels:       msg.Labels,
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

// InsertMessagesWithDedup inserts messages with deduplication using MERGE
// This is more expensive but ensures true upsert semantics
// Use this when you need guaranteed deduplication across longer time windows
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

	// Create ONE temp table for the entire batch
	tempTableName := fmt.Sprintf("temp_email_batch_%d", time.Now().UnixNano())
	dataset := b.client.Dataset(datasetName)
	tempTable := dataset.Table(tempTableName)

	// Create temporary table with same schema
	schema, err := bigquery.InferSchema(EmailRecord{})
	if err != nil {
		return fmt.Errorf("failed to infer schema: %w", err)
	}

	tempMetadata := &bigquery.TableMetadata{
		Schema: schema,
	}

	if err := tempTable.Create(ctx, tempMetadata); err != nil {
		return fmt.Errorf("failed to create temp table: %w", err)
	}

	// Clean up temp table when done
	defer func() {
		if err := tempTable.Delete(ctx); err != nil {
			fmt.Printf("Warning: failed to delete temp table %s: %v\n", tempTableName, err)
		}
	}()

	// Convert all messages to records
	records := make([]*EmailRecord, 0, len(messages))
	for _, msg := range messages {
		records = append(records, &EmailRecord{
			MessageID:    msg.MessageID,
			ThreadID:     msg.ThreadID,
			AccountID:    accountID,
			Sender:       msg.Sender,
			Subject:      msg.Subject,
			BodyText:     msg.BodyText,
			BodyHTML:     msg.BodyHTML,
			BodyMarkdown: msg.BodyMarkdown,
			ReceivedAt:   msg.ReceivedAt,
			IngestedAt:   time.Now(),
			IsRead:       msg.IsRead,
			Labels:       msg.Labels,
		})
	}

	// Insert all records into temp table at once
	inserter := tempTable.Inserter()
	if err := inserter.Put(ctx, records); err != nil {
		return fmt.Errorf("failed to insert into temp table: %w", err)
	}

	// Perform MERGE operation for the entire batch
	mergeQuery := fmt.Sprintf(`
		MERGE `+"`%s.emails`"+` AS target
		USING `+"`%s.%s`"+` AS source
		ON target.message_id = source.message_id AND target.account_id = source.account_id
		WHEN MATCHED THEN
			UPDATE SET
				thread_id = source.thread_id,
				sender = source.sender,
				subject = source.subject,
				body_text = source.body_text,
				body_html = source.body_html,
				body_markdown = source.body_markdown,
				received_at = source.received_at,
				ingested_at = source.ingested_at,
				is_read = source.is_read,
				labels = source.labels
		WHEN NOT MATCHED THEN
			INSERT ROW
	`, datasetName, datasetName, tempTableName)

	query := b.client.Query(mergeQuery)
	query.Location = b.location

	job, err := query.Run(ctx)
	if err != nil {
		return fmt.Errorf("failed to run merge query: %w", err)
	}

	// Wait for the job to complete
	status, err := job.Wait(ctx)
	if err != nil {
		return fmt.Errorf("merge job failed: %w", err)
	}

	if status.Err() != nil {
		return fmt.Errorf("merge job completed with error: %w", status.Err())
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
