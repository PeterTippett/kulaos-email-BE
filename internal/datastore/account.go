package datastore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/google/uuid"
	"github.com/yourusername/email-service/internal/encryption"
)

type AccountStore struct {
	client     *datastore.Client
	kmsService *encryption.KMSService
}

func NewAccountStore(ctx context.Context, projectID string, kmsService *encryption.KMSService) (*AccountStore, error) {
	client, err := datastore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create datastore client: %w", err)
	}
	return &AccountStore{
		client:     client,
		kmsService: kmsService,
	}, nil
}

type EmailAccount struct {
	AccountID         string    `datastore:"account_id"`
	OrgID             string    `datastore:"org_id"`
	EmailAddress      string    `datastore:"email_address"`
	AccessToken       string    `datastore:"access_token,noindex"`  // Encrypted
	RefreshToken      string    `datastore:"refresh_token,noindex"` // Encrypted
	TokenExpiry       time.Time `datastore:"token_expiry"`
	WebhookChannelID  string    `datastore:"webhook_channel_id"`
	WebhookExpiration time.Time `datastore:"webhook_expiration"`
	CreatedAt         time.Time `datastore:"created_at"`
	UpdatedAt         time.Time `datastore:"updated_at"`
}

// EmailAccountLookup is stored in the DEFAULT namespace for routing
type EmailAccountLookup struct {
	EmailAddress string `datastore:"email_address"`
	OrgID        string `datastore:"org_id"`
	Namespace    string `datastore:"namespace"`
	AccountID    string `datastore:"account_id"`
}

func (a *AccountStore) CreateAccount(ctx context.Context, namespace, orgID, emailAddress, accessToken, refreshToken string, tokenExpiry time.Time) (*EmailAccount, error) {
	accountID := uuid.New().String()
	// Normalize email to lowercase for consistent lookup keys
	emailAddress = strings.ToLower(emailAddress)

	// Encrypt tokens using KMS
	encryptedAccess, err := a.kmsService.Encrypt(ctx, orgID, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt access token: %w", err)
	}

	encryptedRefresh, err := a.kmsService.Encrypt(ctx, orgID, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt refresh token: %w", err)
	}

	account := &EmailAccount{
		AccountID:    accountID,
		OrgID:        orgID,
		EmailAddress: emailAddress,
		AccessToken:  encryptedAccess,
		RefreshToken: encryptedRefresh,
		TokenExpiry:  tokenExpiry,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Store in tenant namespace
	key := datastore.NameKey("EmailAccount", accountID, nil)
	key.Namespace = namespace

	if _, err := a.client.Put(ctx, key, account); err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	// Create lookup entry in DEFAULT namespace
	if err := a.createLookupEntry(ctx, emailAddress, orgID, namespace, accountID); err != nil {
		return nil, fmt.Errorf("failed to create lookup entry: %w", err)
	}

	return account, nil
}

func (a *AccountStore) GetAccount(ctx context.Context, namespace, accountID string) (*EmailAccount, error) {
	key := datastore.NameKey("EmailAccount", accountID, nil)
	key.Namespace = namespace

	var account EmailAccount
	if err := a.client.Get(ctx, key, &account); err != nil {
		return nil, err
	}

	return &account, nil
}

// GetAccountWithDecryptedTokens retrieves an account and decrypts its tokens
func (a *AccountStore) GetAccountWithDecryptedTokens(ctx context.Context, namespace, orgID, accountID string) (*EmailAccount, error) {
	account, err := a.GetAccount(ctx, namespace, accountID)
	if err != nil {
		return nil, err
	}

	// Decrypt tokens
	decryptedAccess, err := a.kmsService.Decrypt(ctx, orgID, account.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt access token: %w", err)
	}

	decryptedRefresh, err := a.kmsService.Decrypt(ctx, orgID, account.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt refresh token: %w", err)
	}

	account.AccessToken = decryptedAccess
	account.RefreshToken = decryptedRefresh

	return account, nil
}

func (a *AccountStore) ListAccountsByOrg(ctx context.Context, namespace string) ([]*EmailAccount, error) {
	var accounts []*EmailAccount
	query := datastore.NewQuery("EmailAccount").Namespace(namespace)

	_, err := a.client.GetAll(ctx, query, &accounts)
	if err != nil {
		return nil, err
	}

	return accounts, nil
}

func (a *AccountStore) UpdateTokens(ctx context.Context, namespace, orgID, accountID, accessToken, refreshToken string, tokenExpiry time.Time) error {
	account, err := a.GetAccount(ctx, namespace, accountID)
	if err != nil {
		return err
	}

	// Encrypt new tokens
	encryptedAccess, err := a.kmsService.Encrypt(ctx, orgID, accessToken)
	if err != nil {
		return fmt.Errorf("failed to encrypt access token: %w", err)
	}

	encryptedRefresh, err := a.kmsService.Encrypt(ctx, orgID, refreshToken)
	if err != nil {
		return fmt.Errorf("failed to encrypt refresh token: %w", err)
	}

	account.AccessToken = encryptedAccess
	account.RefreshToken = encryptedRefresh
	account.TokenExpiry = tokenExpiry
	account.UpdatedAt = time.Now()

	key := datastore.NameKey("EmailAccount", accountID, nil)
	key.Namespace = namespace

	_, err = a.client.Put(ctx, key, account)
	return err
}

func (a *AccountStore) UpdateWebhookInfo(ctx context.Context, namespace, accountID, channelID string, expiration time.Time) error {
	account, err := a.GetAccount(ctx, namespace, accountID)
	if err != nil {
		return err
	}

	account.WebhookChannelID = channelID
	account.WebhookExpiration = expiration
	account.UpdatedAt = time.Now()

	key := datastore.NameKey("EmailAccount", accountID, nil)
	key.Namespace = namespace

	_, err = a.client.Put(ctx, key, account)
	return err
}

func (a *AccountStore) GetAccountByEmail(ctx context.Context, emailAddress string) (*EmailAccountLookup, error) {
	// Normalize email to lowercase for consistent lookups
	emailAddress = strings.ToLower(emailAddress)
	key := datastore.NameKey("EmailAccountLookup", emailAddress, nil)
	key.Namespace = DefaultNamespace

	var lookup EmailAccountLookup
	if err := a.client.Get(ctx, key, &lookup); err != nil {
		return nil, err
	}

	return &lookup, nil
}

func (a *AccountStore) createLookupEntry(ctx context.Context, emailAddress, orgID, namespace, accountID string) error {
	lookup := &EmailAccountLookup{
		EmailAddress: emailAddress,
		OrgID:        orgID,
		Namespace:    namespace,
		AccountID:    accountID,
	}

	key := datastore.NameKey("EmailAccountLookup", emailAddress, nil)
	key.Namespace = DefaultNamespace

	_, err := a.client.Put(ctx, key, lookup)
	return err
}

// DeleteAccount removes an account and its lookup entry
func (a *AccountStore) DeleteAccount(ctx context.Context, namespace, accountID, emailAddress string) error {
	// Delete account from tenant namespace
	accountKey := datastore.NameKey("EmailAccount", accountID, nil)
	accountKey.Namespace = namespace

	if err := a.client.Delete(ctx, accountKey); err != nil {
		return fmt.Errorf("failed to delete account: %w", err)
	}

	// Delete lookup entry from DEFAULT namespace
	lookupKey := datastore.NameKey("EmailAccountLookup", emailAddress, nil)
	lookupKey.Namespace = DefaultNamespace

	if err := a.client.Delete(ctx, lookupKey); err != nil {
		return fmt.Errorf("failed to delete lookup entry: %w", err)
	}

	return nil
}

func (a *AccountStore) Close() error {
	return a.client.Close()
}
