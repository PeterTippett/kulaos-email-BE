package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/yourusername/email-service/internal/datastore"
	"github.com/yourusername/email-service/internal/logger"
	"github.com/yourusername/email-service/internal/mcp"
)

// MCPRateLimiter handles rate limiting for MCP endpoints
type MCPRateLimiter struct {
	quotaStore *datastore.MCPQuotaStore
}

// NewMCPRateLimiter creates a new MCP rate limiter
func NewMCPRateLimiter(quotaStore *datastore.MCPQuotaStore) *MCPRateLimiter {
	return &MCPRateLimiter{
		quotaStore: quotaStore,
	}
}

// MCPRequest represents the structure of an MCP request
type MCPRequest struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

// MCPToolCall represents a tool call in an MCP request
type MCPToolCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// getOperationType determines the operation type from the tool name
func getOperationType(toolName string) (datastore.OperationType, bool) {
	switch toolName {
	case "send_email":
		return datastore.OpEmailSend, true
	case "send_sms":
		return datastore.OpSMSSend, true
	case "list_emails", "get_email_details":
		return datastore.OpEmailRead, true
	case "list_sms", "get_sms_details":
		return datastore.OpSMSRead, true
	default:
		return "", false
	}
}

// Middleware is a chi-compatible middleware that enforces rate limits
func (m *MCPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract orgID from context (set by auth middleware)
		orgID := mcp.GetOrgID(ctx)
		if orgID == "" {
			logger.FromContext(ctx).Error("No org ID in context for rate limiting")
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		// For MCP, we need to check the method being called
		// The MCP library will handle the actual request parsing,
		// but we need to extract the tool name for quota checking

		// We'll check quota in the tool handlers themselves rather than here
		// because we need to know the specific tool being called.
		// This middleware can be used for general rate limiting if needed.

		// For now, just pass through to the next handler
		// Quota checking will happen in individual tool handlers
		next.ServeHTTP(w, r)
	})
}

// QuotaErrorResponse represents the error response for quota exceeded
type QuotaErrorResponse struct {
	Error   string                 `json:"error"`
	Code    string                 `json:"code"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// WriteQuotaError writes a quota exceeded error response
func WriteQuotaError(w http.ResponseWriter, opType string, current, limit int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)

	response := QuotaErrorResponse{
		Error: "Quota exceeded",
		Code:  "QUOTA_EXCEEDED",
		Details: map[string]interface{}{
			"operation_type": opType,
			"current_usage":  current,
			"quota_limit":    limit,
			"reset_info":     "Quota resets at the start of each hour",
		},
	}

	json.NewEncoder(w).Encode(response)
}

// CheckQuota checks if an operation is allowed based on quota
// This is a helper function that can be called from tool handlers
func (m *MCPRateLimiter) CheckQuota(ctx context.Context, orgID string, opType datastore.OperationType) (bool, error) {
	return m.quotaStore.CheckQuota(ctx, orgID, opType)
}

// IncrementUsage increments usage for an operation
// This is a helper function that can be called from tool handlers
func (m *MCPRateLimiter) IncrementUsage(ctx context.Context, orgID string, opType datastore.OperationType) error {
	return m.quotaStore.IncrementUsage(ctx, orgID, opType)
}

// GetUsageInfo retrieves current usage information
func (m *MCPRateLimiter) GetUsageInfo(ctx context.Context, orgID string) (*datastore.MCPUsage, error) {
	return m.quotaStore.GetUsage(ctx, orgID)
}
