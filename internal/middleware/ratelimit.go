package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/yourusername/email-service/internal/auth"
	"golang.org/x/time/rate"
)

// RateLimitStrategy defines the type of rate limiting strategy
type RateLimitStrategy int

const (
	// TokenBucket uses golang.org/x/time/rate for token bucket algorithm
	TokenBucket RateLimitStrategy = iota
	// FixedWindow uses a simple time-based window
	FixedWindow
)

// RateLimitConfig holds configuration for rate limiting
type RateLimitConfig struct {
	Strategy          RateLimitStrategy
	Rate              float64       // For TokenBucket: requests per second, For FixedWindow: requests per window
	Burst             int           // For TokenBucket: burst size, For FixedWindow: ignored
	Window            time.Duration // For FixedWindow: window duration, For TokenBucket: ignored
	ErrorMessage      string        // Custom error message
	IncludeRetryAfter bool          // Whether to include Retry-After header
}

// RateLimiter manages per-organization rate limits with configurable strategies
type RateLimiter struct {
	config   RateLimitConfig
	limiters map[string]*rate.Limiter // For TokenBucket strategy
	lastSent map[string]time.Time     // For FixedWindow strategy
	mu       sync.RWMutex
}

// NewRateLimiter creates a new rate limiter with the specified configuration
func NewRateLimiter(config RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		config:   config,
		limiters: make(map[string]*rate.Limiter),
		lastSent: make(map[string]time.Time),
	}
}

// NewTokenBucketRateLimiter creates a token bucket rate limiter (backward compatibility)
func NewTokenBucketRateLimiter(requestsPerSecond float64, burst int) *RateLimiter {
	return NewRateLimiter(RateLimitConfig{
		Strategy:          TokenBucket,
		Rate:              requestsPerSecond,
		Burst:             burst,
		ErrorMessage:      "rate limit exceeded",
		IncludeRetryAfter: false,
	})
}

// NewFixedWindowRateLimiter creates a fixed window rate limiter
func NewFixedWindowRateLimiter(window time.Duration, errorMessage string) *RateLimiter {
	return NewRateLimiter(RateLimitConfig{
		Strategy:          FixedWindow,
		Window:            window,
		ErrorMessage:      errorMessage,
		IncludeRetryAfter: true,
	})
}

// getIdentifier extracts the identifier for rate limiting from the request context
func (rl *RateLimiter) getIdentifier(r *http.Request) string {
	// Extract orgID from context using the auth package's exported key
	// This ensures we use the same typed key that the auth middleware sets
	if orgID := r.Context().Value(auth.OrgIDKey); orgID != nil {
		if orgIDStr, ok := orgID.(string); ok && orgIDStr != "" {
			return orgIDStr
		}
	}

	// Fallback: use remote address for unauthenticated requests
	return r.RemoteAddr
}

// checkTokenBucketLimit checks rate limit using token bucket algorithm
func (rl *RateLimiter) checkTokenBucketLimit(identifier string) (allowed bool, retryAfter time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[identifier]
	if !exists {
		limiter = rate.NewLimiter(rate.Limit(rl.config.Rate), rl.config.Burst)
		rl.limiters[identifier] = limiter
	}

	allowed = limiter.Allow()
	if !allowed {
		// Calculate retry after for token bucket (approximate)
		retryAfter = time.Duration(float64(time.Second) / rl.config.Rate)
	}

	return allowed, retryAfter
}

// checkFixedWindowLimit checks rate limit using fixed window algorithm
func (rl *RateLimiter) checkFixedWindowLimit(identifier string) (allowed bool, retryAfter time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	lastSent, exists := rl.lastSent[identifier]
	now := time.Now()

	if exists && now.Sub(lastSent) < rl.config.Window {
		allowed = false
		retryAfter = rl.config.Window - now.Sub(lastSent)
	} else {
		allowed = true
		rl.lastSent[identifier] = now
	}

	return allowed, retryAfter
}

// Limit is a middleware that enforces rate limiting per organization
func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identifier := rl.getIdentifier(r)

		var allowed bool
		var retryAfter time.Duration

		switch rl.config.Strategy {
		case TokenBucket:
			allowed, retryAfter = rl.checkTokenBucketLimit(identifier)
		case FixedWindow:
			allowed, retryAfter = rl.checkFixedWindowLimit(identifier)
		default:
			// Default to allowing the request
			allowed = true
		}

		if !allowed {
			if rl.config.IncludeRetryAfter {
				w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
			}
			http.Error(w, rl.config.ErrorMessage, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Cleanup removes inactive rate limiters (call periodically to prevent memory leaks)
func (rl *RateLimiter) Cleanup() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()

		if rl.config.Strategy == FixedWindow {
			// Clean up old entries for fixed window
			now := time.Now()
			for identifier, lastSent := range rl.lastSent {
				if now.Sub(lastSent) > time.Hour {
					delete(rl.lastSent, identifier)
				}
			}
		} else {
			// For token bucket, we keep all limiters since they're lightweight
			_ = len(rl.limiters) // Prevent empty critical section warning
		}

		rl.mu.Unlock()
	}
}

// Example usage functions for different rate limiting scenarios

// NewAPIRateLimiter creates a general API rate limiter (token bucket)
func NewAPIRateLimiter(requestsPerSecond float64, burst int) *RateLimiter {
	return NewRateLimiter(RateLimitConfig{
		Strategy:          TokenBucket,
		Rate:              requestsPerSecond,
		Burst:             burst,
		ErrorMessage:      "API rate limit exceeded",
		IncludeRetryAfter: false,
	})
}

// NewMessageSendRateLimiter creates a message send rate limiter for emails and SMS (fixed window)
func NewMessageSendRateLimiter(window time.Duration) *RateLimiter {
	return NewRateLimiter(RateLimitConfig{
		Strategy:          FixedWindow,
		Window:            window,
		ErrorMessage:      "message send rate limit exceeded - please wait before sending another message",
		IncludeRetryAfter: true,
	})
}

// NewEmailSendRateLimiter is deprecated. Use NewMessageSendRateLimiter instead.
// Kept for backward compatibility.
func NewEmailSendRateLimiter(window time.Duration) *RateLimiter {
	return NewMessageSendRateLimiter(window)
}

// NewLoginAttemptRateLimiter creates a login attempt rate limiter (fixed window)
func NewLoginAttemptRateLimiter(window time.Duration) *RateLimiter {
	return NewRateLimiter(RateLimitConfig{
		Strategy:          FixedWindow,
		Window:            window,
		ErrorMessage:      "too many login attempts - please wait before trying again",
		IncludeRetryAfter: true,
	})
}

// NewFileUploadRateLimiter creates a file upload rate limiter (token bucket)
func NewFileUploadRateLimiter(uploadsPerMinute float64, burst int) *RateLimiter {
	return NewRateLimiter(RateLimitConfig{
		Strategy:          TokenBucket,
		Rate:              uploadsPerMinute / 60.0, // Convert to per second
		Burst:             burst,
		ErrorMessage:      "file upload rate limit exceeded",
		IncludeRetryAfter: true,
	})
}
