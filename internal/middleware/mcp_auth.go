package middleware

import (
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/yourusername/email-service/internal/logger"
	"github.com/yourusername/email-service/internal/mcp"
)

// MCPAuthMiddleware handles authentication for MCP endpoints using shared key model
type MCPAuthMiddleware struct {
	sharedKey string
}

// NewMCPAuthMiddleware creates a new MCP authentication middleware
func NewMCPAuthMiddleware(sharedKey string) *MCPAuthMiddleware {
	return &MCPAuthMiddleware{
		sharedKey: sharedKey,
	}
}

// Middleware is a chi-compatible middleware that validates MCP keys
// Key format: orgID-userID-privateKey-scope
// Example: org_1235-kp_gezt-nh4&9dgf#A-email:sending
func (m *MCPAuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Extract Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			logger.FromContext(ctx).Warn("Missing Authorization header")
			http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
			return
		}

		// Extract Bearer token
		if !strings.HasPrefix(authHeader, "Bearer ") {
			logger.FromContext(ctx).Warn("Invalid Authorization header format")
			http.Error(w, "Invalid Authorization header format", http.StatusUnauthorized)
			return
		}

		key := strings.TrimPrefix(authHeader, "Bearer ")
		if key == "" {
			logger.FromContext(ctx).Warn("Empty MCP key")
			http.Error(w, "Empty MCP key", http.StatusUnauthorized)
			return
		}

		// Parse key format: orgID-userID-privateKey-scope
		// Split by first 3 dashes to get the 4 parts (orgID, userID, privateKey, scope)
		parts := strings.SplitN(key, "-", 4)
		if len(parts) != 4 {
			logger.FromContext(ctx).Warn("Invalid MCP key format",
				zap.String("key", key))
			http.Error(w, "Invalid MCP key format (expected: orgID-userID-privateKey-scope)", http.StatusUnauthorized)
			return
		}

		orgID := parts[0]
		userID := parts[1]
		privateKey := parts[2]
		scope := parts[3]

		// Validate private key matches shared secret
		if privateKey != m.sharedKey {
			logger.FromContext(ctx).Warn("Invalid private key",
				zap.String("org_id", orgID),
				zap.String("user_id", userID))
			http.Error(w, "Invalid private key", http.StatusUnauthorized)
			return
		}

		// Validate orgID is not empty
		if orgID == "" {
			logger.FromContext(ctx).Warn("Empty orgID in MCP key")
			http.Error(w, "Invalid MCP key: empty orgID", http.StatusUnauthorized)
			return
		}

		// Add orgID, userID, and scope to context
		ctx = mcp.SetOrgID(ctx, orgID)
		ctx = mcp.SetUserID(ctx, userID)
		ctx = mcp.SetScope(ctx, scope)

		logger.FromContext(ctx).Info("MCP request authenticated",
			zap.String("org_id", orgID),
			zap.String("user_id", userID),
			zap.String("scope", scope))

		// Continue to next handler
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
