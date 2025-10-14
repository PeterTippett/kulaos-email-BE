package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/yourusername/email-service/internal/auth"
	"github.com/yourusername/email-service/internal/datastore"
	"github.com/yourusername/email-service/internal/gmail"
	"github.com/yourusername/email-service/internal/storage"
)

type Handlers struct {
	kindeAuth    *auth.KindeAuth
	gmailOAuth   *gmail.GmailOAuth
	gmailClient  *gmail.GmailClient
	tenantStore  *datastore.TenantStore
	accountStore *datastore.AccountStore
	bqStore      *storage.BigQueryStore
	projectID    string
	pubsubTopic  string
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
) *Handlers {
	return &Handlers{
		kindeAuth:    kindeAuth,
		gmailOAuth:   gmailOAuth,
		gmailClient:  gmailClient,
		tenantStore:  tenantStore,
		accountStore: accountStore,
		bqStore:      bqStore,
		projectID:    projectID,
		pubsubTopic:  pubsubTopic,
	}
}

// Health check endpoint
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

// StartGmailAuth initiates the Gmail OAuth flow
func (h *Handlers) StartGmailAuth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
			log.Printf("Failed to create tenant: %v", err)
			http.Error(w, "failed to create tenant", http.StatusInternalServerError)
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
	ctx := r.Context()

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
		log.Printf("Failed to exchange code: %v", err)
		http.Error(w, "failed to exchange code", http.StatusInternalServerError)
		return
	}

	// Get or create tenant
	tenant, err := h.tenantStore.GetTenant(ctx, orgID)
	if err != nil {
		// Create tenant if doesn't exist
		tenant, err = h.tenantStore.CreateTenant(ctx, orgID)
		if err != nil {
			log.Printf("Failed to create tenant: %v", err)
			http.Error(w, "failed to create tenant", http.StatusInternalServerError)
			return
		}
	}

	// Create email account with encrypted tokens
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
		log.Printf("Failed to create account: %v", err)
		http.Error(w, "failed to create account", http.StatusInternalServerError)
		return
	}

	// Setup Gmail push notifications
	topicName := fmt.Sprintf("projects/%s/topics/%s", h.projectID, h.pubsubTopic)
	watchResp, err := h.gmailClient.SetupWatch(ctx, tokenResp.AccessToken, tokenResp.RefreshToken, topicName)
	if err != nil {
		log.Printf("Failed to setup watch: %v", err)
		// Don't fail - account is still created
	} else {
		// Update account with webhook info
		if err := h.accountStore.UpdateWebhookInfo(ctx, tenant.Namespace, account.AccountID, watchResp.ChannelID, watchResp.Expiration); err != nil {
			log.Printf("Failed to update webhook info: %v", err)
		}
	}

	// Redirect to frontend
	redirectURL := fmt.Sprintf("http://localhost:3005/dashboard?connected=%s", account.AccountID)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// ListAccounts returns the list of connected Gmail accounts for the authenticated org
func (h *Handlers) ListAccounts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
		log.Printf("Failed to list accounts: %v", err)
		http.Error(w, "failed to list accounts", http.StatusInternalServerError)
		return
	}

	// Remove sensitive token data
	sanitizedAccounts := make([]map[string]interface{}, len(accounts))
	for i, acc := range accounts {
		sanitizedAccounts[i] = map[string]interface{}{
			"account_id":    acc.AccountID,
			"email_address": acc.EmailAddress,
			"created_at":    acc.CreatedAt,
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"accounts": sanitizedAccounts,
	})
}

// ListEmails returns recent emails for the authenticated org
func (h *Handlers) ListEmails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
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
		log.Printf("Failed to list emails: %v", err)
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

// DeleteAccount removes an email account and cleans up watch subscriptions
func (h *Handlers) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID := auth.GetOrgID(ctx)

	if orgID == "" {
		http.Error(w, "organization ID not found", http.StatusBadRequest)
		return
	}

	// Ensure DELETE method and extract account ID from path
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Expect path like /api/accounts/{accountId}
	const prefix = "/api/accounts/"
	if !strings.HasPrefix(r.URL.Path, prefix) || len(r.URL.Path) <= len(prefix) {
		http.Error(w, "account ID is required", http.StatusBadRequest)
		return
	}
	accountID := strings.TrimPrefix(r.URL.Path, prefix)

	tenant, err := h.tenantStore.GetTenant(ctx, orgID)
	if err != nil {
		http.Error(w, "tenant not found", http.StatusNotFound)
		return
	}

	// Get account with decrypted tokens to stop watch
	account, err := h.accountStore.GetAccountWithDecryptedTokens(ctx, tenant.Namespace, orgID, accountID)
	if err != nil {
		log.Printf("Failed to get account for deletion: %v", err)
		http.Error(w, "account not found", http.StatusNotFound)
		return
	}

	// Stop Gmail watch subscription
	if err := h.gmailClient.StopWatch(ctx, account.AccessToken, account.RefreshToken); err != nil {
		log.Printf("Warning: Failed to stop watch subscription: %v", err)
		// Continue with deletion even if stopping watch fails
	}

	// Delete account and lookup entry from datastore
	if err := h.accountStore.DeleteAccount(ctx, tenant.Namespace, accountID, account.EmailAddress); err != nil {
		log.Printf("Failed to delete account: %v", err)
		http.Error(w, "failed to delete account", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully deleted account %s (%s)", accountID, account.EmailAddress)
	w.WriteHeader(http.StatusNoContent)
}
