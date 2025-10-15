package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/yourusername/email-service/internal/datastore"
	"github.com/yourusername/email-service/internal/gmail/parser"
	"github.com/yourusername/email-service/internal/storage"
	"github.com/yourusername/email-service/internal/types"
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

	// Log current token state for debugging
	fmt.Printf("🔍 [WEBHOOK] Current token state for %s:\n", account.EmailAddress)
	fmt.Printf("🔍 [WEBHOOK] - Access token length: %d\n", len(account.AccessToken))
	fmt.Printf("🔍 [WEBHOOK] - Refresh token length: %d\n", len(account.RefreshToken))
	fmt.Printf("🔍 [WEBHOOK] - Token expiry: %v\n", account.TokenExpiry)
	fmt.Printf("🔍 [WEBHOOK] - Is token expired: %v\n", h.gmailClient.oauth.IsTokenExpired(account.TokenExpiry))

	// Determine starting history ID
	startHistoryID := account.LastHistoryID
	if startHistoryID == 0 {
		// First notification after watch; use the notification's history as starting point
		startHistoryID = int64(notification.HistoryID)
	}

	// Fetch new messages using history API with paging
	// Use callback to update tokens if they get refreshed
	tokenUpdated := false
	var newAccessToken, newRefreshToken string
	var newExpiry time.Time

	callback := func(accessToken, refreshToken string, expiry time.Time) {
		fmt.Printf("🔄 [WEBHOOK] Token refresh callback triggered for account %s\n", account.EmailAddress)
		fmt.Printf("🔄 [WEBHOOK] New token expiry: %v\n", expiry)
		tokenUpdated = true
		newAccessToken = accessToken
		newRefreshToken = refreshToken
		newExpiry = expiry
	}

	// Create a client with token refresh callback
	ts := h.gmailClient.oauth.GetTokenSourceWithCallback(ctx, account.AccessToken, account.RefreshToken, callback)
	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		log.Printf("⚠️  Failed to create gmail service: %v (acknowledging message)", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Fetch messages using the service
	messages, latestHistoryID, err := h.fetchMessagesWithService(ctx, gmailService, account, startHistoryID)
	if err != nil {
		fmt.Printf("❌ [WEBHOOK] Failed to fetch messages: %v\n", err)

		// If it's an authentication error, try to manually refresh the token
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "invalid authentication") {
			fmt.Printf("🔄 [WEBHOOK] Authentication error detected, attempting manual token refresh\n")

			newToken, refreshErr := h.gmailClient.oauth.RefreshTokenWithRetry(ctx, account.RefreshToken, 3)
			if refreshErr != nil {
				fmt.Printf("❌ [WEBHOOK] Manual token refresh failed: %v\n", refreshErr)
				log.Printf("⚠️  Failed to fetch messages and manual refresh failed: %v (acknowledging message)", err)
				w.WriteHeader(http.StatusOK)
				return
			}

			fmt.Printf("✅ [WEBHOOK] Manual token refresh successful, updating database\n")
			// Update tokens in database
			if updateErr := h.accountStore.UpdateTokens(ctx, lookup.Namespace, lookup.OrgID, lookup.AccountID, newToken.AccessToken, newToken.RefreshToken, newToken.Expiry); updateErr != nil {
				fmt.Printf("❌ [WEBHOOK] Failed to update manually refreshed tokens: %v\n", updateErr)
			}

			// Try fetching messages again with the new token
			fmt.Printf("🔄 [WEBHOOK] Retrying message fetch with refreshed token\n")
			newTS := h.gmailClient.oauth.GetTokenSource(ctx, newToken.AccessToken, newToken.RefreshToken)
			newGmailService, serviceErr := gmail.NewService(ctx, option.WithTokenSource(newTS))
			if serviceErr != nil {
				fmt.Printf("❌ [WEBHOOK] Failed to create new gmail service: %v\n", serviceErr)
				log.Printf("⚠️  Failed to create new gmail service: %v (acknowledging message)", serviceErr)
				w.WriteHeader(http.StatusOK)
				return
			}

			messages, latestHistoryID, err = h.fetchMessagesWithService(ctx, newGmailService, account, startHistoryID)
			if err != nil {
				fmt.Printf("❌ [WEBHOOK] Retry with refreshed token also failed: %v\n", err)
				log.Printf("⚠️  Failed to fetch messages even after token refresh: %v (acknowledging message)", err)
				w.WriteHeader(http.StatusOK)
				return
			}

			fmt.Printf("✅ [WEBHOOK] Successfully fetched messages after manual token refresh\n")
		} else {
			log.Printf("⚠️  Failed to fetch messages: %v (acknowledging message)", err)
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	log.Printf("✓ Fetched %d new messages", len(messages))

	// Update tokens in database if they were refreshed
	if tokenUpdated {
		fmt.Printf("✅ [WEBHOOK] Tokens were refreshed, updating database for account %s\n", account.EmailAddress)
		log.Printf("✓ Tokens were refreshed, updating database")
		if err := h.accountStore.UpdateTokens(ctx, lookup.Namespace, lookup.OrgID, lookup.AccountID, newAccessToken, newRefreshToken, newExpiry); err != nil {
			fmt.Printf("❌ [WEBHOOK] Failed to update refreshed tokens in database: %v\n", err)
			log.Printf("⚠️  Failed to update refreshed tokens: %v", err)
		} else {
			fmt.Printf("✅ [WEBHOOK] Successfully updated refreshed tokens in database\n")
		}
	}

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

// fetchMessagesWithService fetches messages using a Gmail service instance
func (h *WebhookHandler) fetchMessagesWithService(ctx context.Context, gmailService *gmail.Service, account *datastore.EmailAccount, startHistoryID int64) ([]*types.EmailMessage, int64, error) {
	var allMessages []*types.EmailMessage
	var pageToken string
	var latestHistoryID int64 = startHistoryID

	for {
		call := gmailService.Users.History.List("me").StartHistoryId(uint64(latestHistoryID))
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		history, err := call.Do()
		if err != nil {
			return nil, latestHistoryID, fmt.Errorf("failed to fetch history: %w", err)
		}

		for _, hist := range history.History {
			if int64(hist.Id) > latestHistoryID {
				latestHistoryID = int64(hist.Id)
			}
			for _, msg := range hist.MessagesAdded {
				fullMsg, err := gmailService.Users.Messages.Get("me", msg.Message.Id).Format("full").Do()
				if err != nil {
					continue
				}

				// Parse the message
				emailMsg, err := parser.ParseMessage(fullMsg)
				if err != nil {
					continue
				}
				allMessages = append(allMessages, emailMsg)
			}
		}

		if history.NextPageToken == "" {
			break
		}
		pageToken = history.NextPageToken
	}

	return allMessages, latestHistoryID, nil
}
