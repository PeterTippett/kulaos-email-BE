package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	html2text "github.com/k3a/html2text"
	"github.com/yourusername/email-service/internal/types"
)

type BigQueryStore struct {
	client   *bigquery.Client
	location string
}

func NewBigQueryStore(ctx context.Context, projectID, location string) (*BigQueryStore, error) {
	client, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create bigquery client: %w", err)
	}
	return &BigQueryStore{
		client:   client,
		location: location,
	}, nil
}

type EmailRecord struct {
	MessageID      string    `bigquery:"message_id"`
	ThreadID       string    `bigquery:"thread_id"`
	AccountID      string    `bigquery:"account_id"`
	Sender         string    `bigquery:"sender"`
	Subject        string    `bigquery:"subject"`
	BodyText       string    `bigquery:"body_text"`
	ParsedBodyHTML string    `bigquery:"parsed_body_html"`
	ReceivedAt     time.Time `bigquery:"received_at"`
	IngestedAt     time.Time `bigquery:"ingested_at"`
	IsRead         bool      `bigquery:"is_read"`
	Labels         []string  `bigquery:"labels"`
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
		// Table already exists - ensure parsed_body_html exists
		hasParsed := false
		for _, f := range md.Schema {
			if f.Name == "parsed_body_html" {
				hasParsed = true
			}
		}

		if !hasParsed {
			// Add parsed_body_html as a NULLABLE STRING column
			newSchema := append(md.Schema, &bigquery.FieldSchema{
				Name:     "parsed_body_html",
				Type:     bigquery.StringFieldType,
				Required: false,
			})
			update := bigquery.TableMetadataToUpdate{
				Schema: newSchema,
			}
			if _, uerr := table.Update(ctx, update, md.ETag); uerr != nil {
				return fmt.Errorf("failed to update table schema: %w", uerr)
			}
		}
		return nil
	}

	// Infer schema from EmailRecord struct
	schema, err := bigquery.InferSchema(EmailRecord{})
	if err != nil {
		return fmt.Errorf("failed to infer schema: %w", err)
	}

	metadata := &bigquery.TableMetadata{
		Schema: schema,
		TimePartitioning: &bigquery.TimePartitioning{
			Field: "received_at",
		},
	}

	if err := table.Create(ctx, metadata); err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	return nil
}

// InsertMessages inserts multiple email messages into BigQuery
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

	// Convert to BigQuery records
	records := make([]*EmailRecord, len(messages))
	for i, msg := range messages {
		// Derive parsed_body_html from HTML body if present
		var parsedFromHTML string
		if msg.BodyHTML != "" {
			parsedFromHTML = html2text.HTML2Text(msg.BodyHTML)
		}

		records[i] = &EmailRecord{
			MessageID:      msg.MessageID,
			ThreadID:       msg.ThreadID,
			AccountID:      accountID,
			Sender:         msg.Sender,
			Subject:        msg.Subject,
			BodyText:       msg.BodyText,
			ParsedBodyHTML: parsedFromHTML,
			ReceivedAt:     msg.ReceivedAt,
			IngestedAt:     time.Now(),
			IsRead:         msg.IsRead,
			Labels:         msg.Labels,
		}
	}

	// Insert records
	inserter := b.client.Dataset(datasetName).Table("emails").Inserter()
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
			subject,
			body_text,
      parsed_body_html,
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
