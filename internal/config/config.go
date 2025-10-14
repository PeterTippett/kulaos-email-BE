package config

import (
	"fmt"
	"os"

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

func Load() (*Config, error) {
	// Try to load .env file (ignore error if not found)
	_ = godotenv.Load()

	cfg := &Config{
		ProjectID:                    getEnvOrPanic("PROJECT_ID"),
		GoogleApplicationCredentials: os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"),
		KMSLocation:                  getEnvOrDefault("KMS_LOCATION", "global"),
		KMSKeyring:                   getEnvOrPanic("KMS_KEYRING"),
		BigQueryLocation:             getEnvOrDefault("BIGQUERY_LOCATION", "australia-southeast1"),
		KindeDomain:                  getEnvOrPanic("KINDE_DOMAIN"),
		KindeClientID:                getEnvOrPanic("KINDE_CLIENT_ID"),
		KindeClientSecret:            getEnvOrPanic("KINDE_CLIENT_SECRET"),
		KindeRedirectURI:             getEnvOrPanic("KINDE_REDIRECT_URI"),
		GmailClientID:                getEnvOrPanic("GMAIL_CLIENT_ID"),
		GmailClientSecret:            getEnvOrPanic("GMAIL_CLIENT_SECRET"),
		GmailRedirectURI:             getEnvOrPanic("GMAIL_REDIRECT_URI"),
		BackendPort:                  getEnvOrDefault("BACKEND_PORT", "8085"),
		FrontendBaseURL:              getEnvOrPanic("FRONTEND_BASE_URL"),
		PubSubTopic:                  getEnvOrPanic("PUBSUB_TOPIC"),
	}

	return cfg, nil
}

func getEnvOrPanic(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf("required environment variable %s not set", key))
	}
	return value
}

func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
