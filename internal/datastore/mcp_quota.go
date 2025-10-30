package datastore

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/datastore"
)

// MCPQuotaStore manages MCP quotas and usage in Datastore
type MCPQuotaStore struct {
	client *datastore.Client
}

// NewMCPQuotaStore creates a new MCP quota store
func NewMCPQuotaStore(ctx context.Context, projectID string) (*MCPQuotaStore, error) {
	client, err := datastore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create datastore client: %w", err)
	}
	return &MCPQuotaStore{client: client}, nil
}

// MCPQuota represents quota limits for an organization
type MCPQuota struct {
	OrgID                string    `datastore:"org_id"`
	MaxEmailsPerHour     int       `datastore:"max_emails_per_hour"`
	MaxSMSPerHour        int       `datastore:"max_sms_per_hour"`
	MaxEmailReadsPerHour int       `datastore:"max_email_reads_per_hour"`
	MaxSMSReadsPerHour   int       `datastore:"max_sms_reads_per_hour"`
	UpdatedAt            time.Time `datastore:"updated_at"`
}

// MCPUsage represents current usage for an organization in a time window
type MCPUsage struct {
	OrgID       string    `datastore:"org_id"`
	HourWindow  time.Time `datastore:"hour_window"` // Start of the hour window
	EmailsSent  int       `datastore:"emails_sent"`
	SMSSent     int       `datastore:"sms_sent"`
	EmailReads  int       `datastore:"email_reads"`
	SMSReads    int       `datastore:"sms_reads"`
	LastUpdated time.Time `datastore:"last_updated"`
}

// OperationType defines the type of operation for quota checking
type OperationType string

const (
	OpEmailSend OperationType = "email_send"
	OpSMSSend   OperationType = "sms_send"
	OpEmailRead OperationType = "email_read"
	OpSMSRead   OperationType = "sms_read"
)

// Default quota values
const (
	DefaultMaxEmailsPerHour     = 10
	DefaultMaxSMSPerHour        = 10
	DefaultMaxEmailReadsPerHour = 100
	DefaultMaxSMSReadsPerHour   = 100
)

// GetQuota retrieves quota settings for an organization
// If no quota exists, returns default quota
func (s *MCPQuotaStore) GetQuota(ctx context.Context, orgID string) (*MCPQuota, error) {
	key := datastore.NameKey("MCPQuota", orgID, nil)
	key.Namespace = DefaultNamespace

	var quota MCPQuota
	err := s.client.Get(ctx, key, &quota)
	if err == datastore.ErrNoSuchEntity {
		// Return default quota
		return &MCPQuota{
			OrgID:                orgID,
			MaxEmailsPerHour:     DefaultMaxEmailsPerHour,
			MaxSMSPerHour:        DefaultMaxSMSPerHour,
			MaxEmailReadsPerHour: DefaultMaxEmailReadsPerHour,
			MaxSMSReadsPerHour:   DefaultMaxSMSReadsPerHour,
			UpdatedAt:            time.Now(),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get quota: %w", err)
	}

	return &quota, nil
}

// UpdateQuota updates quota settings for an organization
func (s *MCPQuotaStore) UpdateQuota(ctx context.Context, quota *MCPQuota) error {
	key := datastore.NameKey("MCPQuota", quota.OrgID, nil)
	key.Namespace = DefaultNamespace

	quota.UpdatedAt = time.Now()

	if _, err := s.client.Put(ctx, key, quota); err != nil {
		return fmt.Errorf("failed to update quota: %w", err)
	}

	return nil
}

// getCurrentHourWindow returns the start of the current hour window
func getCurrentHourWindow() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location())
}

// getUsageKey generates a Datastore key for usage tracking
func getUsageKey(orgID string, hourWindow time.Time) *datastore.Key {
	// Use orgID + hour window as the key for uniqueness
	keyName := fmt.Sprintf("%s_%d", orgID, hourWindow.Unix())
	key := datastore.NameKey("MCPUsage", keyName, nil)
	key.Namespace = DefaultNamespace
	return key
}

// GetUsage retrieves current usage for an organization
func (s *MCPQuotaStore) GetUsage(ctx context.Context, orgID string) (*MCPUsage, error) {
	hourWindow := getCurrentHourWindow()
	key := getUsageKey(orgID, hourWindow)

	var usage MCPUsage
	err := s.client.Get(ctx, key, &usage)
	if err == datastore.ErrNoSuchEntity {
		// No usage yet for this hour
		return &MCPUsage{
			OrgID:       orgID,
			HourWindow:  hourWindow,
			EmailsSent:  0,
			SMSSent:     0,
			EmailReads:  0,
			SMSReads:    0,
			LastUpdated: time.Now(),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get usage: %w", err)
	}

	return &usage, nil
}

// IncrementUsage increments usage for a specific operation type
func (s *MCPQuotaStore) IncrementUsage(ctx context.Context, orgID string, opType OperationType) error {
	hourWindow := getCurrentHourWindow()
	key := getUsageKey(orgID, hourWindow)

	_, err := s.client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var usage MCPUsage
		err := tx.Get(key, &usage)
		if err == datastore.ErrNoSuchEntity {
			// Initialize new usage record
			usage = MCPUsage{
				OrgID:      orgID,
				HourWindow: hourWindow,
			}
		} else if err != nil {
			return fmt.Errorf("failed to get usage: %w", err)
		}

		// Increment the appropriate counter
		switch opType {
		case OpEmailSend:
			usage.EmailsSent++
		case OpSMSSend:
			usage.SMSSent++
		case OpEmailRead:
			usage.EmailReads++
		case OpSMSRead:
			usage.SMSReads++
		default:
			return fmt.Errorf("unknown operation type: %s", opType)
		}

		usage.LastUpdated = time.Now()

		if _, err := tx.Put(key, &usage); err != nil {
			return fmt.Errorf("failed to update usage: %w", err)
		}

		return nil
	})

	return err
}

// CheckQuota checks if an operation is allowed based on current quota and usage
func (s *MCPQuotaStore) CheckQuota(ctx context.Context, orgID string, opType OperationType) (bool, error) {
	// Get quota settings
	quota, err := s.GetQuota(ctx, orgID)
	if err != nil {
		return false, err
	}

	// Get current usage
	usage, err := s.GetUsage(ctx, orgID)
	if err != nil {
		return false, err
	}

	// Check against appropriate limit
	switch opType {
	case OpEmailSend:
		return usage.EmailsSent < quota.MaxEmailsPerHour, nil
	case OpSMSSend:
		return usage.SMSSent < quota.MaxSMSPerHour, nil
	case OpEmailRead:
		return usage.EmailReads < quota.MaxEmailReadsPerHour, nil
	case OpSMSRead:
		return usage.SMSReads < quota.MaxSMSReadsPerHour, nil
	default:
		return false, fmt.Errorf("unknown operation type: %s", opType)
	}
}

// GetQuotaInfo returns quota and usage information for display
type QuotaInfo struct {
	Quota *MCPQuota
	Usage *MCPUsage
}

// GetQuotaInfo retrieves both quota and usage information
func (s *MCPQuotaStore) GetQuotaInfo(ctx context.Context, orgID string) (*QuotaInfo, error) {
	quota, err := s.GetQuota(ctx, orgID)
	if err != nil {
		return nil, err
	}

	usage, err := s.GetUsage(ctx, orgID)
	if err != nil {
		return nil, err
	}

	return &QuotaInfo{
		Quota: quota,
		Usage: usage,
	}, nil
}

// Close closes the Datastore client
func (s *MCPQuotaStore) Close() error {
	return s.client.Close()
}

