package twilio

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/yourusername/email-service/internal/datastore"
	"github.com/yourusername/email-service/internal/logger"
	"github.com/yourusername/email-service/internal/storage"
	"github.com/yourusername/email-service/internal/types"
)

// WebhookHandler handles incoming SMS webhooks from Twilio
type WebhookHandler struct {
	tenantStore *datastore.TenantStore
	bqStore     *storage.BigQueryStore
}

// NewWebhookHandler creates a new Twilio webhook handler
func NewWebhookHandler(tenantStore *datastore.TenantStore, bqStore *storage.BigQueryStore) *WebhookHandler {
	return &WebhookHandler{
		tenantStore: tenantStore,
		bqStore:     bqStore,
	}
}

// HandleInboundSMS handles incoming SMS webhooks from Twilio
func (h *WebhookHandler) HandleInboundSMS(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	log := logger.FromContext(ctx)

	// Extract org_id from URL path parameter
	orgID := chi.URLParam(r, "org_id")
	if orgID == "" {
		log.Error("Missing org_id in webhook URL")
		http.Error(w, "bad request: missing org_id", http.StatusBadRequest)
		return
	}

	// Parse form data from Twilio webhook
	if err := r.ParseForm(); err != nil {
		log.Error("Failed to parse form data", zap.Error(err))
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	// Extract Twilio webhook parameters
	messageSID := r.FormValue("MessageSid")
	from := r.FormValue("From")
	to := r.FormValue("To")
	body := r.FormValue("Body")
	status := r.FormValue("MessageStatus")

	if messageSID == "" || from == "" || to == "" {
		log.Error("Missing required webhook parameters")
		http.Error(w, "missing required parameters", http.StatusBadRequest)
		return
	}

	log.Info("Received inbound SMS",
		zap.String("org_id", orgID),
		zap.String("message_sid", messageSID),
		zap.String("from", from),
		zap.String("to", to),
		zap.Int("body_length", len(body)))

	// Get tenant by org_id
	tenant, err := h.tenantStore.GetTenant(ctx, orgID)
	if err != nil {
		log.Error("Failed to get tenant", zap.String("org_id", orgID), zap.Error(err))
		http.Error(w, "invalid tenant", http.StatusNotFound)
		return
	}

	// Create SMS message
	message := &types.SMSMessage{
		MessageSID: messageSID,
		From:       from,
		To:         to,
		Body:       body,
		Direction:  "inbound",
		Status:     status,
		ReceivedAt: time.Now(),
		AccountID:  tenant.OrgID, // Use tenant's org ID as account ID
	}

	// Store in BigQuery
	if err := h.bqStore.InsertSMSMessage(ctx, tenant.BigQueryDataset, message, tenant.OrgID); err != nil {
		log.Error("Failed to store SMS in BigQuery",
			zap.String("message_sid", messageSID),
			zap.Error(err))
		http.Error(w, "failed to store message", http.StatusInternalServerError)
		return
	}

	log.Info("Successfully stored inbound SMS",
		zap.String("message_sid", messageSID),
		zap.String("tenant_id", tenant.OrgID))

	// Respond with 200 OK (Twilio expects this for successful processing)
	w.WriteHeader(http.StatusOK)
}

// HandleStatusCallback handles SMS status update webhooks from Twilio
func (h *WebhookHandler) HandleStatusCallback(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	log := logger.FromContext(ctx)

	// Extract org_id from URL path parameter
	orgID := chi.URLParam(r, "org_id")
	if orgID == "" {
		log.Error("Missing org_id in webhook URL")
		http.Error(w, "bad request: missing org_id", http.StatusBadRequest)
		return
	}

	// Parse form data from Twilio webhook
	if err := r.ParseForm(); err != nil {
		log.Error("Failed to parse form data", zap.Error(err))
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	// Extract Twilio status callback parameters
	messageSID := r.FormValue("MessageSid")
	messageStatus := r.FormValue("MessageStatus")

	if messageSID == "" || messageStatus == "" {
		log.Error("Missing required status callback parameters")
		http.Error(w, "missing required parameters", http.StatusBadRequest)
		return
	}

	log.Info("Received SMS status callback",
		zap.String("org_id", orgID),
		zap.String("message_sid", messageSID),
		zap.String("status", messageStatus))

	// Get tenant by org_id
	tenant, err := h.tenantStore.GetTenant(ctx, orgID)
	if err != nil {
		log.Error("Failed to get tenant", zap.String("org_id", orgID), zap.Error(err))
		http.Error(w, "invalid tenant", http.StatusNotFound)
		return
	}

	// Insert status update into status history table
	if err := h.bqStore.InsertSMSStatusHistory(ctx, tenant.BigQueryDataset, messageSID, messageStatus, tenant.OrgID); err != nil {
		log.Error("Failed to insert SMS status history",
			zap.String("message_sid", messageSID),
			zap.String("status", messageStatus),
			zap.Error(err))
		http.Error(w, "failed to record status", http.StatusInternalServerError)
		return
	}

	log.Info("Successfully recorded SMS status update",
		zap.String("message_sid", messageSID),
		zap.String("status", messageStatus))

	// Respond with 200 OK (Twilio expects this for successful processing)
	w.WriteHeader(http.StatusOK)
}
