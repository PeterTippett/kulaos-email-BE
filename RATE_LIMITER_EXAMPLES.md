# Generalized Rate Limiter Examples

This document demonstrates the flexibility and reusability of the new generalized rate limiter implementation.

## Overview

The new `RateLimiter` supports multiple strategies and configurations, eliminating code duplication while providing maximum flexibility.

## Rate Limiting Strategies

### 1. Token Bucket Strategy

- **Algorithm**: Uses `golang.org/x/time/rate` for token bucket
- **Use Cases**: General API rate limiting, file uploads, burst handling
- **Features**: Smooth rate limiting with burst capacity

### 2. Fixed Window Strategy

- **Algorithm**: Simple time-based window
- **Use Cases**: Email sending, login attempts, one-time actions
- **Features**: Precise timing control with Retry-After headers

## Configuration Options

```go
type RateLimitConfig struct {
    Strategy         RateLimitStrategy // TokenBucket or FixedWindow
    Rate             float64           // Requests per second (TokenBucket) or per window (FixedWindow)
    Burst            int               // Burst size (TokenBucket only)
    Window           time.Duration     // Window duration (FixedWindow only)
    ErrorMessage     string            // Custom error message
    IncludeRetryAfter bool            // Whether to include Retry-After header
}
```

## Usage Examples

### 1. General API Rate Limiting (Token Bucket)

```go
// 10 requests per second with burst of 20
apiLimiter := middleware.NewAPIRateLimiter(10, 20)

// Apply to all API routes
r.Use(apiLimiter.Limit)
```

### 2. Message Send Rate Limiting (Fixed Window)

```go
// One message (email or SMS) per 5 seconds
messageLimiter := middleware.NewMessageSendRateLimiter(5 * time.Second)

// Apply to email and SMS send endpoints
r.Group(func(r chi.Router) {
    r.Use(messageLimiter.Limit)
    r.Post("/api/emails/send", handlers.SendEmail)
    r.Post("/api/sms/send", handlers.SendSMS)
})
```

### 3. Login Attempt Rate Limiting (Fixed Window)

```go
// One login attempt per 30 seconds
loginLimiter := middleware.NewLoginAttemptRateLimiter(30 * time.Second)

// Apply to login endpoint
r.Group(func(r chi.Router) {
    r.Use(loginLimiter.Limit)
    r.Post("/auth/login", handlers.Login)
})
```

### 4. File Upload Rate Limiting (Token Bucket)

```go
// 5 uploads per minute with burst of 3
uploadLimiter := middleware.NewFileUploadRateLimiter(5, 3)

// Apply to file upload endpoint
r.Group(func(r chi.Router) {
    r.Use(uploadLimiter.Limit)
    r.Post("/api/files/upload", handlers.UploadFile)
})
```

### 5. Custom Configuration

```go
// Custom rate limiter with specific configuration
customLimiter := middleware.NewRateLimiter(middleware.RateLimitConfig{
    Strategy:         middleware.FixedWindow,
    Window:           5 * time.Minute,
    ErrorMessage:     "too many requests - please wait 5 minutes",
    IncludeRetryAfter: true,
})

// Apply to sensitive endpoint
r.Group(func(r chi.Router) {
    r.Use(customLimiter.Limit)
    r.Post("/api/sensitive-action", handlers.SensitiveAction)
})
```

## Advanced Examples

### Multiple Rate Limiters on Same Endpoint

```go
// Apply both general API limiting and specific message send limiting
r.Group(func(r chi.Router) {
    r.Use(apiLimiter.Limit)        // General API rate limit
    r.Use(messageLimiter.Limit)    // Message send rate limit
    r.Post("/api/emails/send", handlers.SendEmail)
    r.Post("/api/sms/send", handlers.SendSMS)
})
```

### Different Rate Limits for Different User Types

```go
// Premium users get higher limits
premiumLimiter := middleware.NewAPIRateLimiter(100, 200)
freeLimiter := middleware.NewAPIRateLimiter(10, 20)

// Apply based on user type (would need additional middleware)
r.Group(func(r chi.Router) {
    r.Use(userTypeMiddleware)      // Determines user type
    r.Use(rateLimitByUserType)    // Applies appropriate limiter
    r.Get("/api/data", handlers.GetData)
})
```

### Rate Limiting with Custom Error Responses

```go
// Custom error message and retry after
customLimiter := middleware.NewRateLimiter(middleware.RateLimitConfig{
    Strategy:         middleware.FixedWindow,
    Window:           1 * time.Minute,
    ErrorMessage:     "API quota exceeded - please upgrade your plan",
    IncludeRetryAfter: true,
})
```

## Benefits of Generalized Approach

### 1. **DRY Principle**

- Single implementation handles all rate limiting scenarios
- No code duplication between different rate limiters
- Consistent behavior across all endpoints

### 2. **Flexibility**

- Multiple strategies (Token Bucket, Fixed Window)
- Configurable error messages
- Optional Retry-After headers
- Easy to add new strategies

### 3. **Maintainability**

- Single codebase to maintain
- Consistent API across all rate limiters
- Easy to add new features (e.g., Redis support)

### 4. **Performance**

- Efficient algorithms for each strategy
- Minimal memory overhead
- Thread-safe implementation

### 5. **Extensibility**

- Easy to add new strategies (e.g., Sliding Window)
- Configurable cleanup behavior
- Support for different identifier extraction methods

## Migration from Old Implementation

### Before (Duplicated Code)

```go
// Separate implementations for each use case
generalLimiter := middleware.NewRateLimiter(10, 20)
messageLimiter := middleware.NewEmailSendRateLimiter(10 * time.Second)  // Old name
```

### After (Generalized)

```go
// Single implementation with different configurations
generalLimiter := middleware.NewAPIRateLimiter(10, 20)
messageLimiter := middleware.NewMessageSendRateLimiter(10 * time.Second)  // New name
```

## Future Enhancements

### 1. **Redis Support**

```go
// Distributed rate limiting across multiple instances
redisLimiter := middleware.NewRedisRateLimiter(redisClient, config)
```

### 2. **Sliding Window Strategy**

```go
// More sophisticated sliding window algorithm
slidingLimiter := middleware.NewSlidingWindowRateLimiter(window, requests)
```

### 3. **Dynamic Configuration**

```go
// Rate limits that can be updated at runtime
dynamicLimiter := middleware.NewDynamicRateLimiter(configProvider)
```

### 4. **Metrics Integration**

```go
// Built-in metrics and monitoring
metricsLimiter := middleware.NewMetricsRateLimiter(config, metricsCollector)
```

## Conclusion

The generalized rate limiter implementation provides:

- ✅ **Eliminates code duplication**
- ✅ **Supports multiple strategies**
- ✅ **Maintains backward compatibility**
- ✅ **Easy to extend and maintain**
- ✅ **Consistent API across all use cases**
- ✅ **Better performance and memory usage**

This approach follows Go best practices for creating reusable, configurable components while maintaining simplicity and performance.
