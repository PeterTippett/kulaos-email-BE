package mcp

import (
	"context"
	"fmt"
)

// Context keys for MCP
type contextKey string

const (
	// MCPOrgIDKey is the context key for organization ID
	MCPOrgIDKey contextKey = "mcp_org_id"
	// MCPUserIDKey is the context key for user ID
	MCPUserIDKey contextKey = "mcp_user_id"
	// MCPScopeKey is the context key for scope
	MCPScopeKey contextKey = "mcp_scope"
)

// SetOrgID sets the organization ID in the context
func SetOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, MCPOrgIDKey, orgID)
}

// GetOrgID extracts the organization ID from the context
func GetOrgID(ctx context.Context) string {
	if orgID, ok := ctx.Value(MCPOrgIDKey).(string); ok {
		return orgID
	}
	return ""
}

// SetUserID sets the user ID in the context
func SetUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, MCPUserIDKey, userID)
}

// GetUserID extracts the user ID from the context
func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(MCPUserIDKey).(string); ok {
		return userID
	}
	return ""
}

// SetScope sets the scope in the context
func SetScope(ctx context.Context, scope string) context.Context {
	return context.WithValue(ctx, MCPScopeKey, scope)
}

// GetScope extracts the scope from the context
func GetScope(ctx context.Context) string {
	if scope, ok := ctx.Value(MCPScopeKey).(string); ok {
		return scope
	}
	return ""
}

// MCPError represents an MCP-specific error
type MCPError struct {
	Code    string
	Message string
	Details map[string]interface{}
}

func (e *MCPError) Error() string {
	if e.Details != nil && len(e.Details) > 0 {
		return fmt.Sprintf("%s: %s (details: %v)", e.Code, e.Message, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Common MCP errors
var (
	ErrUnauthorized = &MCPError{
		Code:    "UNAUTHORIZED",
		Message: "Invalid or missing API key",
	}
	ErrQuotaExceeded = &MCPError{
		Code:    "QUOTA_EXCEEDED",
		Message: "Quota limit exceeded for this operation",
	}
	ErrInvalidInput = &MCPError{
		Code:    "INVALID_INPUT",
		Message: "Invalid input parameters",
	}
	ErrNotFound = &MCPError{
		Code:    "NOT_FOUND",
		Message: "Resource not found",
	}
	ErrInternal = &MCPError{
		Code:    "INTERNAL_ERROR",
		Message: "Internal server error",
	}
)

// NewQuotaExceededError creates a quota exceeded error with details
func NewQuotaExceededError(opType string, current, limit int) *MCPError {
	return &MCPError{
		Code:    "QUOTA_EXCEEDED",
		Message: fmt.Sprintf("Quota exceeded for %s", opType),
		Details: map[string]interface{}{
			"operation_type": opType,
			"current_usage":  current,
			"quota_limit":    limit,
		},
	}
}
