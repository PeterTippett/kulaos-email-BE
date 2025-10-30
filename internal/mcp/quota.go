package mcp

import (
	"context"
	"fmt"

	"github.com/yourusername/email-service/internal/datastore"
)

// QuotaChecker handles quota checking and usage tracking
type QuotaChecker struct {
	quotaStore *datastore.MCPQuotaStore
}

// NewQuotaChecker creates a new quota checker
func NewQuotaChecker(quotaStore *datastore.MCPQuotaStore) *QuotaChecker {
	return &QuotaChecker{
		quotaStore: quotaStore,
	}
}

// CheckAndIncrement checks quota and increments usage if allowed
func (qc *QuotaChecker) CheckAndIncrement(ctx context.Context, orgID string, opType datastore.OperationType) error {
	// Check if operation is allowed
	allowed, err := qc.quotaStore.CheckQuota(ctx, orgID, opType)
	if err != nil {
		return fmt.Errorf("failed to check quota: %w", err)
	}

	if !allowed {
		// Get current usage for error message
		usage, _ := qc.quotaStore.GetUsage(ctx, orgID)
		quota, _ := qc.quotaStore.GetQuota(ctx, orgID)

		var current, limit int
		switch opType {
		case datastore.OpEmailSend:
			current = usage.EmailsSent
			limit = quota.MaxEmailsPerHour
		case datastore.OpSMSSend:
			current = usage.SMSSent
			limit = quota.MaxSMSPerHour
		case datastore.OpEmailRead:
			current = usage.EmailReads
			limit = quota.MaxEmailReadsPerHour
		case datastore.OpSMSRead:
			current = usage.SMSReads
			limit = quota.MaxSMSReadsPerHour
		}

		return NewQuotaExceededError(string(opType), current, limit)
	}

	// Increment usage
	if err := qc.quotaStore.IncrementUsage(ctx, orgID, opType); err != nil {
		return fmt.Errorf("failed to increment usage: %w", err)
	}

	return nil
}

// GetQuotaInfo retrieves quota and usage information
func (qc *QuotaChecker) GetQuotaInfo(ctx context.Context, orgID string) (*datastore.QuotaInfo, error) {
	return qc.quotaStore.GetQuotaInfo(ctx, orgID)
}

