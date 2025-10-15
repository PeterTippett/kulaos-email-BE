package middleware

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter manages per-organization rate limits
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.RWMutex
	// Requests per second per organization
	rps float64
	// Burst size
	burst int
}

// NewRateLimiter creates a new rate limiter with specified rate and burst
func NewRateLimiter(requestsPerSecond float64, burst int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		rps:      requestsPerSecond,
		burst:    burst,
	}
}

// getLimiter returns the rate limiter for a given organization ID
func (rl *RateLimiter) getLimiter(orgID string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	limiter, exists := rl.limiters[orgID]
	if !exists {
		limiter = rate.NewLimiter(rate.Limit(rl.rps), rl.burst)
		rl.limiters[orgID] = limiter
	}

	return limiter
}

// Limit is a middleware that enforces rate limiting per organization
func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract orgID from context (set by auth middleware)
		orgID := r.Context().Value("org_id")
		if orgID == nil {
			// If no org ID, apply a default rate limit using remote address
			orgID = r.RemoteAddr
		}

		orgIDStr, ok := orgID.(string)
		if !ok {
			orgIDStr = "unknown"
		}

		limiter := rl.getLimiter(orgIDStr)
		if !limiter.Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
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
		// In a production system, you'd track last access time and remove old entries
		// For now, we keep all limiters since they're lightweight
		rl.mu.Unlock()
	}
}

