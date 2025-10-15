package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"

	golangjwt "github.com/golang-jwt/jwt/v5"
	"github.com/kinde-oss/kinde-go/jwt"
	"github.com/yourusername/email-service/internal/logger"
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
	jwksURL      string
}

func NewKindeAuth(domain, clientID, clientSecret string) *KindeAuth {
	// Construct JWKS URL from domain
	jwksURL := fmt.Sprintf("https://%s/.well-known/jwks", domain)

	return &KindeAuth{
		domain:       domain,
		clientID:     clientID,
		clientSecret: clientSecret,
		jwksURL:      jwksURL,
	}
}

// Middleware is a chi-compatible middleware that extracts and validates Kinde JWT token
func (k *KindeAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "missing authorization header", http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			http.Error(w, "invalid authorization header format", http.StatusUnauthorized)
			return
		}

		// Parse and validate token with JWKS verification
		// Note: We validate 'azp' (authorized party) claim instead of 'aud' because
		// Kinde may use azp when the token doesn't have a specific audience
		parsedToken, err := jwt.ParseFromString(
			tokenString,
			jwt.WillValidateWithJWKSUrl(k.jwksURL),
			jwt.WillValidateIssuer(fmt.Sprintf("https://%s", k.domain)),
			jwt.WillValidateAlgorithm("RS256"),
			// Custom validation for azp claim
			jwt.WillValidateClaims(func(claims golangjwt.MapClaims) (bool, error) {
				// Check azp (authorized party) claim
				azp, ok := claims["azp"].(string)
				if ok && azp == k.clientID {
					return true, nil
				}

				// Fall back to checking aud if azp is not present
				if aud, ok := claims["aud"].([]interface{}); ok {
					for _, a := range aud {
						if audStr, ok := a.(string); ok && audStr == k.clientID {
							return true, nil
						}
					}
				}
				if audStr, ok := claims["aud"].(string); ok && audStr == k.clientID {
					return true, nil
				}

				return false, fmt.Errorf("token must have azp or aud claim matching client ID")
			}),
		)

		if err != nil {
			logger.Get().Warn("Token validation failed", zap.Error(err))
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		// If ParseFromString succeeded, the token is valid
		// No need to check GetValidationErrors() again as it's already handled by ParseFromString

		// Extract claims
		claims := parsedToken.GetClaims()
		if claims == nil {
			http.Error(w, "missing token claims", http.StatusUnauthorized)
			return
		}

		// Extract user ID (sub claim)
		sub, ok := claims["sub"].(string)
		if !ok || sub == "" {
			http.Error(w, "missing sub claim", http.StatusUnauthorized)
			return
		}

		// Extract email (may not always be present)
		email, _ := claims["email"].(string)

		// Extract organization code (org_code claim)
		orgCode, _ := claims["org_code"].(string)
		if orgCode == "" {
			http.Error(w, "user must be in an organization", http.StatusForbidden)
			return
		}

		// Attach claims to context
		ctx := context.WithValue(r.Context(), OrgIDKey, orgCode)
		ctx = context.WithValue(ctx, UserIDKey, sub)
		ctx = context.WithValue(ctx, UserEmailKey, email)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuth is a handler wrapper for backward compatibility
func (k *KindeAuth) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		k.Middleware(next).ServeHTTP(w, r)
	}
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
