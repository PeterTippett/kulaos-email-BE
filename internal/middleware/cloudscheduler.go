package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/yourusername/email-service/internal/logger"
)

// CloudSchedulerAuth middleware verifies requests from Cloud Scheduler
// It validates the OIDC token provided by Cloud Scheduler
type CloudSchedulerAuth struct {
	projectID string
	// For additional security, you can also check specific service account
	allowedServiceAccount string
}

// NewCloudSchedulerAuth creates a new Cloud Scheduler authentication middleware
func NewCloudSchedulerAuth(projectID, allowedServiceAccount string) *CloudSchedulerAuth {
	return &CloudSchedulerAuth{
		projectID:             projectID,
		allowedServiceAccount: allowedServiceAccount,
	}
}

// Middleware validates the OIDC token from Cloud Scheduler
func (c *CloudSchedulerAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		// Get the Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			log.Warn("Missing Authorization header for Cloud Scheduler endpoint")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Extract the token
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			log.Warn("Invalid Authorization header format")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Parse the JWT token (without verification - we trust the token from Cloud Scheduler)
		// In production, you should verify the token signature against Google's public keys
		token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
		if err != nil {
			log.Error("Failed to parse OIDC token", zap.Error(err))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			log.Warn("Invalid token claims")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Verify the issuer is Google
		issuer, ok := claims["iss"].(string)
		if !ok || (issuer != "https://accounts.google.com" && issuer != "accounts.google.com") {
			log.Warn("Invalid token issuer", zap.String("issuer", issuer))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Verify the email matches our expected service account (if configured)
		if c.allowedServiceAccount != "" {
			email, ok := claims["email"].(string)
			if !ok || subtle.ConstantTimeCompare([]byte(email), []byte(c.allowedServiceAccount)) != 1 {
				log.Warn("Token email does not match allowed service account",
					zap.String("token_email", email),
					zap.String("allowed", c.allowedServiceAccount))
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		// Verify the token hasn't expired
		if exp, ok := claims["exp"].(float64); ok {
			expiryTime := time.Unix(int64(exp), 0)
			if time.Now().After(expiryTime) {
				log.Warn("Token has expired", zap.Time("expiry", expiryTime))
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		// Add claims to context for potential use in handlers
		ctx = context.WithValue(ctx, "scheduler_claims", claims)

		log.Info("Cloud Scheduler authentication successful")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SimpleTokenAuth provides a simpler alternative using a shared secret token
// Use this if you want a simpler authentication mechanism (less secure than OIDC)
type SimpleTokenAuth struct {
	token string
}

// NewSimpleTokenAuth creates a new simple token authentication middleware
func NewSimpleTokenAuth(token string) *SimpleTokenAuth {
	return &SimpleTokenAuth{token: token}
}

// Middleware validates a simple bearer token
func (s *SimpleTokenAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		// Get the token from header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// Also check X-Cloudscheduler header as a fallback
			schedulerHeader := r.Header.Get("X-Cloudscheduler")
			if schedulerHeader != "true" {
				log.Warn("Missing authentication headers")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
				return
			}

			// For development/testing: allow requests with X-Cloudscheduler header
			// In production, you should remove this and require proper authentication
			log.Warn("Request authenticated via X-Cloudscheduler header (development mode)")
			next.ServeHTTP(w, r)
			return
		}

		// Extract the token
		providedToken := strings.TrimPrefix(authHeader, "Bearer ")

		// Constant-time comparison to prevent timing attacks
		if subtle.ConstantTimeCompare([]byte(providedToken), []byte(s.token)) != 1 {
			log.Warn("Invalid authentication token")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "Unauthorized"})
			return
		}

		log.Info("Token authentication successful")
		next.ServeHTTP(w, r)
	})
}

// GetSchedulerClaims retrieves the Cloud Scheduler OIDC claims from the request context
func GetSchedulerClaims(ctx context.Context) (jwt.MapClaims, error) {
	claims, ok := ctx.Value("scheduler_claims").(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("no scheduler claims in context")
	}
	return claims, nil
}
