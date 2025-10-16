package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/yourusername/email-service/internal/api"
	"github.com/yourusername/email-service/internal/auth"
	"github.com/yourusername/email-service/internal/config"
	"github.com/yourusername/email-service/internal/datastore"
	"github.com/yourusername/email-service/internal/encryption"
	"github.com/yourusername/email-service/internal/gmail"
	"github.com/yourusername/email-service/internal/logger"
	internalMiddleware "github.com/yourusername/email-service/internal/middleware"
	"github.com/yourusername/email-service/internal/storage"
)

func main() {
	ctx := context.Background()

	// Initialize logger
	isDev := os.Getenv("ENVIRONMENT") != "production"
	if err := logger.Init(isDev); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Get().Fatal("Failed to load config", zap.Error(err))
	}

	logger.Get().Info("Starting email service", zap.String("port", cfg.BackendPort))

	// Initialize KMS service
	kmsService, err := encryption.NewKMSService(ctx, cfg.ProjectID, cfg.KMSLocation, cfg.KMSKeyring)
	if err != nil {
		logger.Get().Fatal("Failed to create KMS service", zap.Error(err))
	}
	defer kmsService.Close()

	// Initialize Datastore
	tenantStore, err := datastore.NewTenantStore(ctx, cfg.ProjectID)
	if err != nil {
		logger.Get().Fatal("Failed to create tenant store", zap.Error(err))
	}
	defer tenantStore.Close()

	accountStore, err := datastore.NewAccountStore(ctx, cfg.ProjectID, kmsService)
	if err != nil {
		logger.Get().Fatal("Failed to create account store", zap.Error(err))
	}
	defer accountStore.Close()

	// Initialize BigQuery
	bqStore, err := storage.NewBigQueryStore(ctx, cfg.ProjectID, cfg.BigQueryLocation)
	if err != nil {
		logger.Get().Fatal("Failed to create BigQuery store", zap.Error(err))
	}
	defer bqStore.Close()

	// Initialize Kinde auth
	kindeAuth := auth.NewKindeAuth(cfg.KindeDomain, cfg.KindeClientID, cfg.KindeClientSecret)

	// Initialize Gmail services
	gmailOAuth := gmail.NewGmailOAuth(cfg.GmailClientID, cfg.GmailClientSecret, cfg.GmailRedirectURI)
	gmailClient := gmail.NewGmailClient(gmailOAuth)

	// Initialize webhook handler
	webhookHandler := gmail.NewWebhookHandler(gmailClient, accountStore, tenantStore, bqStore)

	// Initialize API handlers
	handlers := api.NewHandlers(kindeAuth, gmailOAuth, gmailClient, tenantStore, accountStore, bqStore, cfg.ProjectID, cfg.PubSubTopic, cfg.FrontendBaseURL)

	// Initialize rate limiter (10 requests per second, burst of 20)
	rateLimiter := internalMiddleware.NewRateLimiter(10, 20)

	// Setup chi router with middleware chain
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID) // Add request ID to context
	r.Use(middleware.RealIP)    // Get real client IP
	r.Use(requestIDMiddleware)  // Add request ID to logger context
	r.Use(loggingMiddleware)    // Log all requests
	r.Use(middleware.Recoverer) // Recover from panics
	// Configure CORS to allow requests only from the configured frontend base URL
	allowedOrigins := []string{}
	if cfg.FrontendBaseURL != "" {
		allowedOrigins = append(allowedOrigins, cfg.FrontendBaseURL)
	}
	r.Use(corsMiddleware(allowedOrigins))
	r.Use(rateLimiter.Limit) // Rate limiting per organization

	// Public routes
	r.Get("/health", handlers.Health)
	r.Get("/auth/gmail/callback", handlers.GmailCallback)
	r.Post("/webhooks/gmail", webhookHandler.HandleGmailWebhook)
	// Cloud Scheduler endpoint for refreshing Gmail watch subscriptions
	// TODO: Add authentication middleware to ensure only Cloud Scheduler can call this
	// See: https://cloud.google.com/scheduler/docs/http-target-auth
	r.Post("/cron/refresh-watches", handlers.RefreshWatchSubscriptions)

	// Protected routes (require Kinde authentication)
	r.Group(func(r chi.Router) {
		r.Use(kindeAuth.Middleware)

		r.Get("/auth/gmail/start", handlers.StartGmailAuth)
		r.Get("/api/accounts", handlers.ListAccounts)
		r.Get("/api/emails", handlers.ListEmails)
		r.Delete("/api/accounts/{accountID}", handlers.DeleteAccount)
	})

	// Create server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.BackendPort),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Get().Info("Server listening", zap.String("port", cfg.BackendPort))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Get().Fatal("Server failed", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Get().Info("Shutting down server")

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Get().Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Get().Info("Server stopped")
}

// requestIDMiddleware extracts request ID from chi middleware and adds it to logger context
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := middleware.GetReqID(r.Context())
		ctx := logger.WithRequestID(r.Context(), requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// loggingMiddleware logs every HTTP request and response
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		// Process the request
		next.ServeHTTP(ww, r)

		// Log the response with compact format
		duration := time.Since(start)
		logger.FromContext(r.Context()).Info(
			fmt.Sprintf("%s %s %d %s", r.Method, r.URL.Path, ww.Status(), duration.Round(time.Millisecond)),
		)
	})
}

// corsMiddleware returns a middleware that adds CORS headers allowing only the given origins
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	// Build a quick lookup map for allowed origins
	originAllowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originAllowed[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Only set CORS for allowed origins
			if origin != "" {
				if _, ok := originAllowed[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				}
			}

			// Handle preflight requests early
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
