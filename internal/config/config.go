package config

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/joho/godotenv"
)

type Config struct {
	// GCP
	ProjectID                    string
	GoogleApplicationCredentials string

	// KMS
	KMSLocation string
	KMSKeyring  string

	// BigQuery
	BigQueryLocation string

	// Kinde
	KindeDomain       string
	KindeClientID     string
	KindeClientSecret string

	// Gmail
	GmailClientID     string
	GmailClientSecret string

	// Backend
	BackendPort    string
	BackendBaseURL string

	// Frontend
	FrontendBaseURL string

	// Pub/Sub
	PubSubTopic string

	// Twilio
	TwilioAccountSID  string
	TwilioAuthToken   string
	TwilioPhoneNumber string

	// MCP
	MCPEnabled   bool
	MCPSharedKey string
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func Load() (*Config, error) {
	ctx := context.Background()

	// Only load .env file in development (not on App Engine)
	// App Engine sets GAE_ENV, GAE_APPLICATION, or GOOGLE_CLOUD_PROJECT
	if os.Getenv("GAE_ENV") == "" && os.Getenv("GOOGLE_CLOUD_PROJECT") == "" {
		_ = godotenv.Load() // Ignore error if .env not found
	}

	projectID := os.Getenv("PROJECT_ID")

	// Fetch secrets - first try direct env vars, then Secret Manager
	kindeClientID, err := getEnvOrSecretManager(ctx, projectID, "KINDE_CLIENT_ID", "KINDE_CLIENT_ID_SECRET_NAME")
	if err != nil {
		return nil, fmt.Errorf("failed to load KINDE_CLIENT_ID: %w", err)
	}

	kindeClientSecret, err := getEnvOrSecretManager(ctx, projectID, "KINDE_CLIENT_SECRET", "KINDE_CLIENT_SECRET_SECRET_NAME")
	if err != nil {
		return nil, fmt.Errorf("failed to load KINDE_CLIENT_SECRET: %w", err)
	}

	gmailClientID, err := getEnvOrSecretManager(ctx, projectID, "GMAIL_CLIENT_ID", "GMAIL_CLIENT_ID_SECRET_NAME")
	if err != nil {
		return nil, fmt.Errorf("failed to load GMAIL_CLIENT_ID: %w", err)
	}

	gmailClientSecret, err := getEnvOrSecretManager(ctx, projectID, "GMAIL_CLIENT_SECRET", "GMAIL_CLIENT_SECRET_SECRET_NAME")
	if err != nil {
		return nil, fmt.Errorf("failed to load GMAIL_CLIENT_SECRET: %w", err)
	}

	twilioAccountSID, err := getEnvOrSecretManager(ctx, projectID, "TWILIO_ACCOUNT_SID", "TWILIO_ACCOUNT_SID_SECRET_NAME")
	if err != nil {
		return nil, fmt.Errorf("failed to load TWILIO_ACCOUNT_SID: %w", err)
	}

	twilioAuthToken, err := getEnvOrSecretManager(ctx, projectID, "TWILIO_AUTH_TOKEN", "TWILIO_AUTH_TOKEN_SECRET_NAME")
	if err != nil {
		return nil, fmt.Errorf("failed to load TWILIO_AUTH_TOKEN: %w", err)
	}

	mcpSharedKey, err := getEnvOrSecretManager(ctx, projectID, "MCP_SHARED_KEY", "MCP_SHARED_KEY_SECRET_NAME")
	if err != nil {
		return nil, fmt.Errorf("failed to load MCP_SHARED_KEY: %w", err)
	}

	cfg := &Config{
		ProjectID:                    projectID,
		GoogleApplicationCredentials: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
		KMSLocation:                  getEnvOrDefault("KMS_LOCATION", "global"),
		KMSKeyring:                   os.Getenv("KMS_KEYRING"),
		BigQueryLocation:             getEnvOrDefault("BIGQUERY_LOCATION", "australia-southeast1"),
		KindeDomain:                  os.Getenv("KINDE_DOMAIN"),
		KindeClientID:                kindeClientID,
		KindeClientSecret:            kindeClientSecret,
		GmailClientID:                gmailClientID,
		GmailClientSecret:            gmailClientSecret,
		BackendPort:                  getEnvOrDefault("BACKEND_PORT", "8085"),
		BackendBaseURL:               os.Getenv("BACKEND_BASE_URL"),
		FrontendBaseURL:              os.Getenv("FRONTEND_BASE_URL"),
		PubSubTopic:                  os.Getenv("PUBSUB_TOPIC"),
		TwilioAccountSID:             twilioAccountSID,
		TwilioAuthToken:              twilioAuthToken,
		TwilioPhoneNumber:            os.Getenv("TWILIO_PHONE_NUMBER"),
		MCPEnabled:                   getEnvOrDefault("MCP_ENABLED", "true") == "true",
		MCPSharedKey:                 mcpSharedKey,
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that all required configuration values are present and valid
func (c *Config) Validate() error {
	var errors []ValidationError

	// Required string fields
	requiredFields := map[string]string{
		"PROJECT_ID":          c.ProjectID,
		"KMS_KEYRING":         c.KMSKeyring,
		"KINDE_DOMAIN":        c.KindeDomain,
		"KINDE_CLIENT_ID":     c.KindeClientID,
		"KINDE_CLIENT_SECRET": c.KindeClientSecret,
		"GMAIL_CLIENT_ID":     c.GmailClientID,
		"GMAIL_CLIENT_SECRET": c.GmailClientSecret,
		"BACKEND_BASE_URL":    c.BackendBaseURL,
		"FRONTEND_BASE_URL":   c.FrontendBaseURL,
		"PUBSUB_TOPIC":        c.PubSubTopic,
		"TWILIO_ACCOUNT_SID":  c.TwilioAccountSID,
		"TWILIO_AUTH_TOKEN":   c.TwilioAuthToken,
		"TWILIO_PHONE_NUMBER": c.TwilioPhoneNumber,
	}

	for field, value := range requiredFields {
		if value == "" {
			errors = append(errors, ValidationError{
				Field:   field,
				Message: "is required but not set",
			})
		}
	}

	// Validate GCP Project ID format (lowercase letters, numbers, hyphens)
	if c.ProjectID != "" {
		if !isValidGCPProjectID(c.ProjectID) {
			errors = append(errors, ValidationError{
				Field:   "PROJECT_ID",
				Message: "must contain only lowercase letters, numbers, and hyphens (e.g., 'my-project-123')",
			})
		}
	}

	// Validate Kinde domain format (should not include https://)
	if c.KindeDomain != "" {
		if strings.HasPrefix(c.KindeDomain, "http://") || strings.HasPrefix(c.KindeDomain, "https://") {
			errors = append(errors, ValidationError{
				Field:   "KINDE_DOMAIN",
				Message: "should not include protocol (e.g., 'your-tenant.kinde.com', not 'https://your-tenant.kinde.com')",
			})
		}
		if strings.Contains(c.KindeDomain, "your-tenant") {
			errors = append(errors, ValidationError{
				Field:   "KINDE_DOMAIN",
				Message: "contains placeholder value 'your-tenant' - please set your actual Kinde domain",
			})
		}
	}

	// Validate URLs (redirect URIs are constructed from BACKEND_BASE_URL, so no need to validate separately)
	urlFields := map[string]string{
		"BACKEND_BASE_URL":  c.BackendBaseURL,
		"FRONTEND_BASE_URL": c.FrontendBaseURL,
	}

	for field, value := range urlFields {
		if value != "" && !isValidURL(value) {
			errors = append(errors, ValidationError{
				Field:   field,
				Message: fmt.Sprintf("'%s' is not a valid URL (e.g., 'http://localhost:3000')", value),
			})
		}
	}

	// Validate port number
	if c.BackendPort != "" {
		if port, err := strconv.Atoi(c.BackendPort); err != nil || port < 1 || port > 65535 {
			errors = append(errors, ValidationError{
				Field:   "BACKEND_PORT",
				Message: fmt.Sprintf("'%s' is not a valid port number (must be 1-65535)", c.BackendPort),
			})
		}
	}

	// Check for placeholder values
	placeholderChecks := map[string]string{
		"KINDE_CLIENT_ID": "your-kinde-client-id",
		"GMAIL_CLIENT_ID": "your-gmail-oauth-client-id",
		"PROJECT_ID":      "your-gcp-project-id",
	}

	for field, placeholder := range placeholderChecks {
		var value string
		switch field {
		case "KINDE_CLIENT_ID":
			value = c.KindeClientID
		case "GMAIL_CLIENT_ID":
			value = c.GmailClientID
		case "PROJECT_ID":
			value = c.ProjectID
		}
		if strings.Contains(value, placeholder) {
			errors = append(errors, ValidationError{
				Field:   field,
				Message: fmt.Sprintf("contains placeholder value '%s' - please set your actual value", placeholder),
			})
		}
	}

	if len(errors) > 0 {
		return &ConfigValidationError{Errors: errors}
	}

	return nil
}

// ConfigValidationError represents multiple validation errors
type ConfigValidationError struct {
	Errors []ValidationError
}

func (e *ConfigValidationError) Error() string {
	var sb strings.Builder
	sb.WriteString("configuration validation failed:\n")
	for _, err := range e.Errors {
		sb.WriteString(fmt.Sprintf("  - %s\n", err.Error()))
	}
	sb.WriteString("\nPlease check your .env file and environment variables.")
	return sb.String()
}

// isValidGCPProjectID checks if a string is a valid GCP project ID
func isValidGCPProjectID(projectID string) bool {
	// GCP project IDs must:
	// - Be 6-30 characters
	// - Start with lowercase letter
	// - Contain only lowercase letters, numbers, and hyphens
	// - Not end with hyphen
	pattern := `^[a-z][a-z0-9-]{4,28}[a-z0-9]$`
	matched, _ := regexp.MatchString(pattern, projectID)
	return matched
}

// isValidURL checks if a string is a valid URL
func isValidURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.Scheme != "" && u.Host != ""
}

func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// fetchSecretFromGCP fetches a secret from Google Secret Manager
func fetchSecretFromGCP(ctx context.Context, projectID, secretName string) (string, error) {
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create secret manager client: %w", err)
	}
	defer client.Close()

	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", projectID, secretName)
	result, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	})
	if err != nil {
		return "", fmt.Errorf("failed to access secret %s: %w", secretName, err)
	}

	return string(result.Payload.Data), nil
}

// getEnvOrSecretManager gets a value from env var directly, or from Secret Manager if *_SECRET_NAME is set
func getEnvOrSecretManager(ctx context.Context, projectID, directEnvKey, secretNameEnvKey string) (string, error) {
	// First, try direct environment variable (for local dev)
	if value := os.Getenv(directEnvKey); value != "" {
		return value, nil
	}

	// Otherwise, check for secret name in *_SECRET_NAME env var
	secretName := os.Getenv(secretNameEnvKey)
	if secretName == "" {
		return "", nil // Not configured
	}

	// Fetch from Secret Manager
	return fetchSecretFromGCP(ctx, projectID, secretName)
}
