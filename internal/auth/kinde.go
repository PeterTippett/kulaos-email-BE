package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type contextKey string

const (
	OrgIDKey     contextKey = "org_id"
	UserIDKey    contextKey = "user_id"
	UserEmailKey contextKey = "user_email"
)

type KindeAuth struct {
	domain       string
	clientID     string
	clientSecret string
}

func NewKindeAuth(domain, clientID, clientSecret string) *KindeAuth {
	return &KindeAuth{
		domain:       domain,
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

// KindeClaims represents the claims we extract from Kinde JWT
type KindeClaims struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	OrgCode string `json:"org_code"` // Kinde uses org_code as a string
	OrgID   string // Will be populated from org_code
}

// RequireAuth middleware that extracts and validates Kinde JWT token
func (k *KindeAuth) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			http.Error(w, "invalid authorization header format", http.StatusUnauthorized)
			return
		}

		// In production, you'd validate the JWT signature
		// For POC, we'll decode it without full validation
		claims, err := k.extractClaims(token)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid token: %v", err), http.StatusUnauthorized)
			return
		}

		if claims.OrgID == "" {
			http.Error(w, "user must be in an organization", http.StatusForbidden)
			return
		}

		// Attach claims to context
		ctx := context.WithValue(r.Context(), OrgIDKey, claims.OrgID)
		ctx = context.WithValue(ctx, UserIDKey, claims.Sub)
		ctx = context.WithValue(ctx, UserEmailKey, claims.Email)

		next(w, r.WithContext(ctx))
	}
}

// extractClaims extracts claims from JWT token
// Note: This is a simplified version for POC. In production, use a proper JWT library
// and validate the signature against Kinde's JWKS endpoint
func (k *KindeAuth) extractClaims(token string) (*KindeClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	// Decode payload (middle part)
	payload := parts[1]

	// Try URL-safe base64 decoding with padding first
	var decoded []byte
	var err error

	// Add padding if needed
	if l := len(payload) % 4; l > 0 {
		payload += strings.Repeat("=", 4-l)
	}

	// Try URL-safe encoding first (standard for JWTs)
	decoded, err = base64.URLEncoding.DecodeString(payload)
	if err != nil {
		// If that fails, try RawURLEncoding
		payload = parts[1] // reset payload without padding
		decoded, err = base64.RawURLEncoding.DecodeString(payload)
		if err != nil {
			// If that also fails, try standard base64
			payload = parts[1]
			if l := len(payload) % 4; l > 0 {
				payload += strings.Repeat("=", 4-l)
			}
			decoded, err = base64.StdEncoding.DecodeString(payload)
			if err != nil {
				return nil, fmt.Errorf("failed to decode token with all methods: %w", err)
			}
		}
	}

	var claims KindeClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse claims: %w", err)
	}

	// Set OrgID from org_code
	claims.OrgID = claims.OrgCode

	return &claims, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetOrgID extracts the organization ID from the request context
func GetOrgID(ctx context.Context) string {
	if orgID, ok := ctx.Value(OrgIDKey).(string); ok {
		return orgID
	}
	return ""
}

// GetUserID extracts the user ID from the request context
func GetUserID(ctx context.Context) string {
	if userID, ok := ctx.Value(UserIDKey).(string); ok {
		return userID
	}
	return ""
}

// GetUserEmail extracts the user email from the request context
func GetUserEmail(ctx context.Context) string {
	if email, ok := ctx.Value(UserEmailKey).(string); ok {
		return email
	}
	return ""
}
