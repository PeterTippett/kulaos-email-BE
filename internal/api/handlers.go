package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/google/uuid"
	"github.com/yourusername/email-service/internal/auth"
	"github.com/yourusername/email-service/internal/datastore"
	"github.com/yourusername/email-service/internal/gmail"
	"github.com/yourusername/email-service/internal/logger"
	"github.com/yourusername/email-service/internal/storage"
)

type Handlers struct {
	kindeAuth       *auth.KindeAuth
	gmailOAuth      *gmail.GmailOAuth
	gmailClient     *gmail.GmailClient
	tenantStore     *datastore.TenantStore
	accountStore    *datastore.AccountStore
	bqStore         *storage.BigQueryStore
	projectID       string
	pubsubTopic     string
	frontendBaseURL string
}

func NewHandlers(
	kindeAuth *auth.KindeAuth,
	gmailOAuth *gmail.GmailOAuth,
	gmailClient *gmail.GmailClient,
	tenantStore *datastore.TenantStore,
	accountStore *datastore.AccountStore,
	bqStore *storage.BigQueryStore,
	projectID string,
	pubsubTopic string,
	frontendBaseURL string,
) *Handlers {
	return &Handlers{
		kindeAuth:       kindeAuth,
		gmailOAuth:      gmailOAuth,
		gmailClient:     gmailClient,
		tenantStore:     tenantStore,
		accountStore:    accountStore,
		bqStore:         bqStore,
		projectID:       projectID,
		pubsubTopic:     pubsubTopic,
		frontendBaseURL: frontendBaseURL,
	}
}

// Health check endpoint
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// StartGmailAuth initiates the Gmail OAuth flow
func (h *Handlers) StartGmailAuth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	orgID := auth.GetOrgID(ctx)

	if orgID == "" {
		http.Error(w, "organization ID not found in token", http.StatusBadRequest)
		return
	}

	// Ensure tenant exists
	_, err := h.tenantStore.GetTenant(ctx, orgID)
	if err != nil {
		// Create tenant if doesn't exist
		_, err = h.tenantStore.CreateTenant(ctx, orgID)
		if err != nil {
			logger.FromContext(ctx).Error("Failed to create tenant",
				zap.String("org_id", orgID),
				zap.Error(err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	// Generate state token with org ID
	state := fmt.Sprintf("%s:%s", orgID, uuid.New().String())

	// Get auth URL
	authURL := h.gmailOAuth.GetAuthURL(state)

	json.NewEncoder(w).Encode(map[string]string{
		"auth_url": authURL,
		"state":    state,
	})
}

// GmailCallback handles the OAuth callback from Gmail
func (h *Handlers) GmailCallback(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		http.Error(w, "missing code or state", http.StatusBadRequest)
		return
	}

	// Extract org ID from state (format: orgID:uuid)
	orgID := state
	if idx := strings.Index(state, ":"); idx != -1 {
		orgID = state[:idx]
	}

	if orgID == "" {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}

	// Exchange code for tokens
	tokenResp, err := h.gmailOAuth.ExchangeCode(ctx, code)
	if err != nil {
		logger.FromContext(ctx).Error("Failed to exchange code",
			zap.String("org_id", orgID),
			zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Get or create tenant
	tenant, err := h.tenantStore.GetTenant(ctx, orgID)
	if err != nil {
		// Create tenant if doesn't exist
		tenant, err = h.tenantStore.CreateTenant(ctx, orgID)
		if err != nil {
			logger.FromContext(ctx).Error("Failed to create tenant",
				zap.String("org_id", orgID),
				zap.Error(err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	// Normalize email for consistent lookups
	tokenResp.Email = strings.ToLower(tokenResp.Email)

	// Either update existing account in this tenant or create a new one
	var accountID string
	if lookup, lerr := h.accountStore.GetAccountByEmail(ctx, tokenResp.Email); lerr == nil {
		// Email already known
		// Ensure it belongs to this tenant; if not, reject
		if lookup.Namespace != tenant.Namespace {
			logger.FromContext(ctx).Warn("Email already connected to different tenant",
				zap.String("email", tokenResp.Email),
				zap.String("namespace", lookup.Namespace))
			http.Error(w, "email already connected to another organization", http.StatusConflict)
			return
		}

		// Update tokens on existing account
		if err := h.accountStore.UpdateTokens(
			ctx,
			tenant.Namespace,
			tenant.OrgID,
			lookup.AccountID,
			tokenResp.AccessToken,
			tokenResp.RefreshToken,
			tokenResp.Expiry,
		); err != nil {
			logger.FromContext(ctx).Error("Failed to update tokens",
				zap.String("account_id", lookup.AccountID),
				zap.Error(err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		accountID = lookup.AccountID
	} else {
		// Create new account
		account, err := h.accountStore.CreateAccount(
			ctx,
			tenant.Namespace,
			tenant.OrgID,
			tokenResp.Email,
			tokenResp.AccessToken,
			tokenResp.RefreshToken,
			tokenResp.Expiry,
		)
		if err != nil {
			logger.FromContext(ctx).Error("Failed to create account",
				zap.String("email", tokenResp.Email),
				zap.Error(err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		accountID = account.AccountID
	}

	// Setup Gmail push notifications
	topicName := fmt.Sprintf("projects/%s/topics/%s", h.projectID, h.pubsubTopic)
	watchResp, err := h.gmailClient.SetupWatch(ctx, tokenResp.AccessToken, tokenResp.RefreshToken, topicName)
	if err != nil {
		logger.FromContext(ctx).Warn("Failed to setup watch", zap.Error(err))
		// Don't fail - account is still created
	} else {
		// Update account with webhook info
		if err := h.accountStore.UpdateWebhookInfo(ctx, tenant.Namespace, accountID, watchResp.ChannelID, watchResp.Expiration); err != nil {
			logger.FromContext(ctx).Warn("Failed to update webhook info", zap.Error(err))
		}
		// Record starting history ID for incremental sync
		if err := h.accountStore.UpdateLastHistoryID(ctx, tenant.Namespace, accountID, watchResp.StartHistoryID); err != nil {
			logger.FromContext(ctx).Warn("Failed to update starting history ID", zap.Error(err))
		}
	}

	// Redirect to frontend
	redirectURL := fmt.Sprintf("%s/dashboard?connected=%s", h.frontendBaseURL, accountID)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// ListAccounts returns the list of connected Gmail accounts for the authenticated org
func (h *Handlers) ListAccounts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	orgID := auth.GetOrgID(ctx)

	if orgID == "" {
		http.Error(w, "organization ID not found", http.StatusBadRequest)
		return
	}

	tenant, err := h.tenantStore.GetTenant(ctx, orgID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accounts": []interface{}{},
		})
		return
	}

	accounts, err := h.accountStore.ListAccountsByOrg(ctx, tenant.Namespace)
	if err != nil {
		logger.FromContext(ctx).Error("Failed to list accounts",
			zap.String("org_id", orgID),
			zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Remove sensitive token data
	sanitizedAccounts := make([]map[string]interface{}, len(accounts))
	for i, acc := range accounts {
		sanitizedAccounts[i] = map[string]interface{}{
			"account_id":    acc.AccountID,
			"email_address": acc.EmailAddress,
			"status":        acc.Status,
			"created_at":    acc.CreatedAt,
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"accounts": sanitizedAccounts,
	})
}

// ListEmails returns recent emails for the authenticated org
func (h *Handlers) ListEmails(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	orgID := auth.GetOrgID(ctx)

	if orgID == "" {
		http.Error(w, "organization ID not found", http.StatusBadRequest)
		return
	}

	tenant, err := h.tenantStore.GetTenant(ctx, orgID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"emails": []interface{}{},
		})
		return
	}

	// Query BigQuery for recent emails
	emails, err := h.bqStore.ListEmails(ctx, tenant.BigQueryDataset, 50)
	if err != nil {
		logger.FromContext(ctx).Warn("Failed to list emails", zap.Error(err))
		// Return empty list if dataset doesn't exist yet
		json.NewEncoder(w).Encode(map[string]interface{}{
			"emails": []interface{}{},
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"emails": emails,
	})
}

// DeleteAccount disconnects an email account (soft delete) and cleans up watch subscriptions
func (h *Handlers) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	orgID := auth.GetOrgID(ctx)

	if orgID == "" {
		http.Error(w, "organization ID not found", http.StatusBadRequest)
		return
	}

	// Extract account ID from URL parameter (chi router)
	accountID := chi.URLParam(r, "accountID")
	if accountID == "" {
		http.Error(w, "account ID is required", http.StatusBadRequest)
		return
	}

	tenant, err := h.tenantStore.GetTenant(ctx, orgID)
	if err != nil {
		http.Error(w, "tenant not found", http.StatusNotFound)
		return
	}

	// Get account with decrypted tokens to stop watch
	account, err := h.accountStore.GetAccountWithDecryptedTokens(ctx, tenant.Namespace, orgID, accountID)
	if err != nil {
		logger.FromContext(ctx).Error("Failed to get account for disconnection",
			zap.String("account_id", accountID),
			zap.Error(err))
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}

	// Stop Gmail watch subscription (only if account is active and has tokens)
	if account.Status == "active" && account.AccessToken != "" {
		if err := h.gmailClient.StopWatch(ctx, account.AccessToken, account.RefreshToken); err != nil {
			logger.FromContext(ctx).Warn("Failed to stop watch subscription", zap.Error(err))
			// Continue with disconnection even if stopping watch fails
		}
	}

	// Perform soft delete: mark as disconnected and clear sensitive tokens
	if err := h.accountStore.DisconnectAccount(ctx, tenant.Namespace, accountID); err != nil {
		logger.FromContext(ctx).Error("Failed to disconnect account",
			zap.String("account_id", accountID),
			zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	logger.FromContext(ctx).Info("Successfully disconnected account",
		zap.String("account_id", accountID),
		zap.String("email", account.EmailAddress))
	w.WriteHeader(http.StatusNoContent)
}

// RefreshWatchSubscriptions checks for expiring Gmail watch subscriptions and renews them
// This endpoint is designed to be called by Cloud Scheduler on a regular basis (e.g., daily)
func (h *Handlers) RefreshWatchSubscriptions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	log := logger.FromContext(ctx)
	log.Info("🔄 Starting watch subscription refresh job")

	// Get all active accounts to check for expiring watches
	// We check all accounts rather than just expiring ones to handle cases where
	// webhook_expiration might be null or incorrectly set
	accounts, err := h.accountStore.GetAllActiveAccounts(ctx)
	if err != nil {
		log.Error("Failed to get active accounts", zap.Error(err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	log.Info("Found active accounts to check", zap.Int("count", len(accounts)))

	// Filter for accounts that need watch refresh
	// Refresh if: expiration is within 2 days OR expiration is null/not set
	refreshThreshold := time.Now().Add(48 * time.Hour)
	var accountsToRefresh []*datastore.EmailAccount

	for _, account := range accounts {
		needsRefresh := false
		if account.WebhookExpiration == nil {
			log.Info("Account has no webhook expiration set, will refresh",
				zap.String("email", account.EmailAddress),
				zap.String("account_id", account.AccountID))
			needsRefresh = true
		} else if account.WebhookExpiration.Before(refreshThreshold) {
			log.Info("Account webhook expiring soon, will refresh",
				zap.String("email", account.EmailAddress),
				zap.String("account_id", account.AccountID),
				zap.Time("expiration", *account.WebhookExpiration))
			needsRefresh = true
		}

		if needsRefresh {
			accountsToRefresh = append(accountsToRefresh, account)
		}
	}

	log.Info("Accounts needing watch refresh", zap.Int("count", len(accountsToRefresh)))

	refreshedCount := 0
	failedCount := 0
	var refreshErrors []string

	// Refresh watches for each account
	for _, account := range accountsToRefresh {
		if err := h.refreshAccountWatch(ctx, account); err != nil {
			log.Error("Failed to refresh watch for account",
				zap.String("email", account.EmailAddress),
				zap.String("account_id", account.AccountID),
				zap.Error(err))
			failedCount++
			refreshErrors = append(refreshErrors, fmt.Sprintf("%s: %v", account.EmailAddress, err))
		} else {
			log.Info("Successfully refreshed watch for account",
				zap.String("email", account.EmailAddress),
				zap.String("account_id", account.AccountID))
			refreshedCount++
		}
	}

	// Return summary
	response := map[string]interface{}{
		"total_accounts":      len(accounts),
		"accounts_to_refresh": len(accountsToRefresh),
		"refreshed":           refreshedCount,
		"failed":              failedCount,
	}

	if len(refreshErrors) > 0 {
		response["errors"] = refreshErrors
	}

	log.Info("Watch subscription refresh job completed",
		zap.Int("total", len(accounts)),
		zap.Int("to_refresh", len(accountsToRefresh)),
		zap.Int("refreshed", refreshedCount),
		zap.Int("failed", failedCount))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// refreshAccountWatch refreshes the Gmail watch subscription for a single account
func (h *Handlers) refreshAccountWatch(ctx context.Context, account *datastore.EmailAccount) error {
	// Get tenant info to find namespace
	tenant, err := h.tenantStore.GetTenant(ctx, account.OrgID)
	if err != nil {
		return fmt.Errorf("failed to get tenant: %w", err)
	}

	// Get account with decrypted tokens
	decryptedAccount, err := h.accountStore.GetAccountWithDecryptedTokens(ctx, tenant.Namespace, account.OrgID, account.AccountID)
	if err != nil {
		return fmt.Errorf("failed to get decrypted tokens: %w", err)
	}

	// Setup new watch
	topicName := fmt.Sprintf("projects/%s/topics/%s", h.projectID, h.pubsubTopic)
	watchResp, err := h.gmailClient.SetupWatch(ctx, decryptedAccount.AccessToken, decryptedAccount.RefreshToken, topicName)
	if err != nil {
		return fmt.Errorf("failed to setup watch: %w", err)
	}

	// Update account with new webhook info
	if err := h.accountStore.UpdateWebhookInfo(ctx, tenant.Namespace, account.AccountID, watchResp.ChannelID, watchResp.Expiration); err != nil {
		return fmt.Errorf("failed to update webhook info: %w", err)
	}

	fmt.Printf("✅ Refreshed watch for %s - new expiration: %v\n", account.EmailAddress, watchResp.Expiration)
	return nil
}
