package gmail

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/yourusername/email-service/internal/datastore"
	"github.com/yourusername/email-service/internal/storage"
)

type WebhookHandler struct {
	gmailClient  *GmailClient
	accountStore *datastore.AccountStore
	tenantStore  *datastore.TenantStore
	bqStore      *storage.BigQueryStore
}

func NewWebhookHandler(
	gmailClient *GmailClient,
	accountStore *datastore.AccountStore,
	tenantStore *datastore.TenantStore,
	bqStore *storage.BigQueryStore,
) *WebhookHandler {
	return &WebhookHandler{
		gmailClient:  gmailClient,
		accountStore: accountStore,
		tenantStore:  tenantStore,
		bqStore:      bqStore,
	}
}

type GmailNotification struct {
	EmailAddress string `json:"emailAddress"`
	HistoryID    uint64 `json:"historyId"`
}

type PubSubMessage struct {
	Message struct {
		Data        string `json:"data"`
		MessageID   string `json:"messageId"`
		PublishTime string `json:"publishTime"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// HandleGmailWebhook processes Gmail push notifications
func (h *WebhookHandler) HandleGmailWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Failed to read body: %v", err)
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var pubsubMsg PubSubMessage
	if err := json.Unmarshal(body, &pubsubMsg); err != nil {
		log.Printf("Failed to parse message: %v", err)
		http.Error(w, "failed to parse message", http.StatusBadRequest)
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(pubsubMsg.Message.Data)
	if err != nil {
		log.Printf("Failed to decode data: %v", err)
		http.Error(w, "failed to decode data", http.StatusBadRequest)
		return
	}

	var notification GmailNotification
	if err := json.Unmarshal(decoded, &notification); err != nil {
		log.Printf("Failed to parse notification: %v", err)
		http.Error(w, "failed to parse notification", http.StatusBadRequest)
		return
	}

	log.Printf("Received notification for email: %s, historyID: %d", notification.EmailAddress, notification.HistoryID)

	// Look up which tenant owns this email address
	lookup, err := h.accountStore.GetAccountByEmail(ctx, notification.EmailAddress)
	if err != nil {
		log.Printf("⚠️  Failed to find tenant for email %s: %v (acknowledging message to prevent retry)", notification.EmailAddress, err)
		// Acknowledge the message to prevent infinite retries
		// This could be a test message or an account that was disconnected
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Printf("✓ Routing email to org: %s (namespace: %s)", lookup.OrgID, lookup.Namespace)

	// Get tenant info
	tenant, err := h.tenantStore.GetTenant(ctx, lookup.OrgID)
	if err != nil {
		log.Printf("⚠️  Failed to get tenant %s: %v (acknowledging message)", lookup.OrgID, err)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Get account with decrypted tokens
	account, err := h.accountStore.GetAccountWithDecryptedTokens(ctx, lookup.Namespace, lookup.OrgID, lookup.AccountID)
	if err != nil {
		log.Printf("⚠️  Failed to get account: %v (acknowledging message)", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Determine starting history ID
	startHistoryID := account.LastHistoryID
	if startHistoryID == 0 {
		// First notification after watch; use the notification's history as starting point
		startHistoryID = int64(notification.HistoryID)
	}

	// Fetch new messages using history API with paging
	messages, latestHistoryID, err := h.gmailClient.FetchNewMessagesPaged(ctx, account.AccessToken, account.RefreshToken, startHistoryID)
	if err != nil {
		log.Printf("⚠️  Failed to fetch messages: %v (acknowledging message)", err)
		// Acknowledge anyway to prevent retries
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Printf("✓ Fetched %d new messages", len(messages))

	// Store messages in tenant's BigQuery dataset
	if len(messages) > 0 {
		if err := h.bqStore.InsertMessages(ctx, tenant.BigQueryDataset, messages, account.AccountID); err != nil {
			log.Printf("❌ Failed to store messages in BigQuery: %v", err)
		} else {
			log.Printf("✓ Stored %d messages in BigQuery dataset: %s", len(messages), tenant.BigQueryDataset)
		}

		// Persist the latest history ID for incremental sync
		if latestHistoryID > 0 {
			if err := h.accountStore.UpdateLastHistoryID(ctx, lookup.Namespace, lookup.AccountID, latestHistoryID); err != nil {
				log.Printf("⚠️  Failed to update last history ID: %v", err)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}
