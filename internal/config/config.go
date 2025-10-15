package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

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
	KindeRedirectURI  string

	// Gmail
	GmailClientID     string
	GmailClientSecret string
	GmailRedirectURI  string

	// Backend
	BackendPort string

	// Frontend
	FrontendBaseURL string

	// Pub/Sub
	PubSubTopic string
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
	// Try to load .env file (ignore error if not found)
	_ = godotenv.Load()

	cfg := &Config{
		ProjectID:                    os.Getenv("PROJECT_ID"),
		GoogleApplicationCredentials: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
		KMSLocation:                  getEnvOrDefault("KMS_LOCATION", "global"),
		KMSKeyring:                   os.Getenv("KMS_KEYRING"),
		BigQueryLocation:             getEnvOrDefault("BIGQUERY_LOCATION", "australia-southeast1"),
		KindeDomain:                  os.Getenv("KINDE_DOMAIN"),
		KindeClientID:                os.Getenv("KINDE_CLIENT_ID"),
		KindeClientSecret:            os.Getenv("KINDE_CLIENT_SECRET"),
		KindeRedirectURI:             os.Getenv("KINDE_REDIRECT_URI"),
		GmailClientID:                os.Getenv("GMAIL_CLIENT_ID"),
		GmailClientSecret:            os.Getenv("GMAIL_CLIENT_SECRET"),
		GmailRedirectURI:             os.Getenv("GMAIL_REDIRECT_URI"),
		BackendPort:                  getEnvOrDefault("BACKEND_PORT", "8085"),
		FrontendBaseURL:              os.Getenv("FRONTEND_BASE_URL"),
		PubSubTopic:                  os.Getenv("PUBSUB_TOPIC"),
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
		"KINDE_REDIRECT_URI":  c.KindeRedirectURI,
		"GMAIL_CLIENT_ID":     c.GmailClientID,
		"GMAIL_CLIENT_SECRET": c.GmailClientSecret,
		"GMAIL_REDIRECT_URI":  c.GmailRedirectURI,
		"FRONTEND_BASE_URL":   c.FrontendBaseURL,
		"PUBSUB_TOPIC":        c.PubSubTopic,
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

	// Validate URLs
	urlFields := map[string]string{
		"KINDE_REDIRECT_URI": c.KindeRedirectURI,
		"GMAIL_REDIRECT_URI": c.GmailRedirectURI,
		"FRONTEND_BASE_URL":  c.FrontendBaseURL,
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
