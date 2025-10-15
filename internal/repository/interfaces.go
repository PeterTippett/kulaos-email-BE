package repository

import (
	"context"
	"time"

	"github.com/yourusername/email-service/internal/datastore"
	"github.com/yourusername/email-service/internal/storage"
	"github.com/yourusername/email-service/internal/types"
)

// AccountRepository defines the interface for account data operations
type AccountRepository interface {
	// CreateAccount creates a new email account with encrypted tokens
	CreateAccount(ctx context.Context, namespace, orgID, emailAddress, accessToken, refreshToken string, tokenExpiry time.Time) (*datastore.EmailAccount, error)

	// GetAccount retrieves an account by its ID
	GetAccount(ctx context.Context, namespace, accountID string) (*datastore.EmailAccount, error)

	// GetAccountWithDecryptedTokens retrieves an account with decrypted OAuth tokens
	GetAccountWithDecryptedTokens(ctx context.Context, namespace, orgID, accountID string) (*datastore.EmailAccount, error)

	// GetAccountByEmail looks up an account by email address
	GetAccountByEmail(ctx context.Context, emailAddress string) (*datastore.EmailAccountLookup, error)

	// ListAccountsByOrg lists all accounts for an organization
	ListAccountsByOrg(ctx context.Context, namespace string) ([]*datastore.EmailAccount, error)

	// UpdateTokens updates OAuth tokens for an account
	UpdateTokens(ctx context.Context, namespace, orgID, accountID, accessToken, refreshToken string, tokenExpiry time.Time) error

	// UpdateWebhookInfo updates Gmail watch webhook information
	UpdateWebhookInfo(ctx context.Context, namespace, accountID, channelID string, expiration time.Time) error

	// UpdateLastHistoryID updates the last processed Gmail history ID
	UpdateLastHistoryID(ctx context.Context, namespace, accountID string, historyID int64) error

	// DeleteAccount removes an account and its lookup entry
	DeleteAccount(ctx context.Context, namespace, accountID, emailAddress string) error

	// Close closes the repository connection
	Close() error
}

// TenantRepository defines the interface for tenant data operations
type TenantRepository interface {
	// CreateTenant creates a new tenant with a unique namespace
	CreateTenant(ctx context.Context, orgID string) (*datastore.Tenant, error)

	// GetTenant retrieves a tenant by organization ID
	GetTenant(ctx context.Context, orgID string) (*datastore.Tenant, error)

	// Close closes the repository connection
	Close() error
}

// EmailStorage defines the interface for email data storage operations
type EmailStorage interface {
	// CreateTenantDataset creates a BigQuery dataset for a tenant
	CreateTenantDataset(ctx context.Context, datasetName string) error

	// EnsureEmailTable creates the emails table if it doesn't exist
	EnsureEmailTable(ctx context.Context, datasetName string) error

	// InsertMessages inserts multiple email messages using streaming inserts
	InsertMessages(ctx context.Context, datasetName string, messages []*types.EmailMessage, accountID string) error

	// InsertMessagesWithDedup inserts messages with guaranteed deduplication using MERGE
	InsertMessagesWithDedup(ctx context.Context, datasetName string, messages []*types.EmailMessage, accountID string) error

	// ListEmails retrieves recent emails for a tenant
	ListEmails(ctx context.Context, datasetName string, limit int) ([]*storage.EmailRecord, error)

	// Close closes the storage connection
	Close() error
}

// EncryptionService defines the interface for encryption operations
type EncryptionService interface {
	// Encrypt encrypts data using KMS
	Encrypt(ctx context.Context, namespace, plaintext string) (string, error)

	// Decrypt decrypts data using KMS
	Decrypt(ctx context.Context, namespace, ciphertext string) (string, error)

	// Close closes the encryption service
	Close() error
}

// Ensure concrete types implement interfaces at compile time
var (
	_ AccountRepository = (*datastore.AccountStore)(nil)
	_ TenantRepository  = (*datastore.TenantStore)(nil)
	_ EmailStorage      = (*storage.BigQueryStore)(nil)
)
