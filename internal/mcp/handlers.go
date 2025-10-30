package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	mcpSDK "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"

	"github.com/yourusername/email-service/internal/datastore"
	"github.com/yourusername/email-service/internal/gmail"
	"github.com/yourusername/email-service/internal/logger"
	"github.com/yourusername/email-service/internal/storage"
	"github.com/yourusername/email-service/internal/types"
)

// MCPHandlers contains all MCP handlers and their dependencies
type MCPHandlers struct {
	gmailClient  *gmail.GmailClient
	twilioClient TwilioClient
	tenantStore  *datastore.TenantStore
	accountStore *datastore.AccountStore
	bqStore      *storage.BigQueryStore
	quotaChecker *QuotaChecker
}

// TwilioClient interface for sending SMS
type TwilioClient interface {
	SendSMS(ctx context.Context, to, body, orgID string) (*types.SMSMessage, error)
}

// NewMCPHandlers creates a new MCP handlers instance
func NewMCPHandlers(
	gmailClient *gmail.GmailClient,
	twilioClient TwilioClient,
	tenantStore *datastore.TenantStore,
	accountStore *datastore.AccountStore,
	bqStore *storage.BigQueryStore,
	quotaChecker *QuotaChecker,
) *MCPHandlers {
	return &MCPHandlers{
		gmailClient:  gmailClient,
		twilioClient: twilioClient,
		tenantStore:  tenantStore,
		accountStore: accountStore,
		bqStore:      bqStore,
		quotaChecker: quotaChecker,
	}
}

// ==================== RESOURCE HANDLERS (Read Operations) ====================

// HandleEmailsList handles the emails://list resource
// Supports query parameters: page, per_page, account_id
func (h *MCPHandlers) HandleEmailsList(ctx context.Context, req *mcpSDK.ReadResourceRequest) (*mcpSDK.ReadResourceResult, error) {
	orgID := GetOrgID(ctx)
	if orgID == "" {
		return nil, fmt.Errorf("organization ID not found in context")
	}

	// Check quota
	if err := h.quotaChecker.CheckAndIncrement(ctx, orgID, datastore.OpEmailRead); err != nil {
		logger.FromContext(ctx).Warn("Email read quota exceeded", zap.String("org_id", orgID))
		return nil, err
	}

	// Parse query parameters
	params, err := parseQueryParams(req.Params.URI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URI: %w", err)
	}

	page := getIntQueryParam(params, "page", 1)
	perPage := getIntQueryParam(params, "per_page", 20)
	accountID := params.Get("account_id")

	// Validate pagination
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 50 {
		perPage = 20
	}

	// Validate accountID format if provided (UUIDs only)
	if accountID != "" {
		// Simple UUID format validation
		if len(accountID) != 36 || accountID[8] != '-' || accountID[13] != '-' || accountID[18] != '-' || accountID[23] != '-' {
			return nil, fmt.Errorf("invalid account_id format")
		}
	}

	// Get tenant
	tenant, err := h.tenantStore.GetTenant(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("tenant not found: %w", err)
	}

	// Query emails using paginated method with optional account filter
	result, err := h.bqStore.ListEmailsPaginatedWithFilter(ctx, tenant.BigQueryDataset, page, perPage, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to list emails: %w", err)
	}

	jsonData, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return &mcpSDK.ReadResourceResult{
		Contents: []*mcpSDK.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(jsonData),
			},
		},
	}, nil
}

// HandleEmailDetails handles the emails://message/{messageId} resource
func (h *MCPHandlers) HandleEmailDetails(ctx context.Context, req *mcpSDK.ReadResourceRequest) (*mcpSDK.ReadResourceResult, error) {
	orgID := GetOrgID(ctx)
	if orgID == "" {
		return nil, fmt.Errorf("organization ID not found in context")
	}

	// Check quota
	if err := h.quotaChecker.CheckAndIncrement(ctx, orgID, datastore.OpEmailRead); err != nil {
		logger.FromContext(ctx).Warn("Email read quota exceeded", zap.String("org_id", orgID))
		return nil, err
	}

	// Extract message ID from URI (emails://message/{messageId})
	parts := strings.Split(req.Params.URI, "/")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid URI format, expected emails://message/{messageId}")
	}
	messageID := parts[len(parts)-1]

	if messageID == "" {
		return nil, fmt.Errorf("message ID is required")
	}

	// Get tenant
	tenant, err := h.tenantStore.GetTenant(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("tenant not found: %w", err)
	}

	// Get email details - for now, list recent emails and filter
	// TODO: Add GetEmailByMessageID method to BigQueryStore for better performance
	emails, err := h.bqStore.ListEmails(ctx, tenant.BigQueryDataset, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to query emails: %w", err)
	}

	// Find the specific email
	var email *storage.EmailRecord
	for _, e := range emails {
		if e.MessageID == messageID {
			email = e
			break
		}
	}

	if email == nil {
		return nil, mcpSDK.ResourceNotFoundError(req.Params.URI)
	}

	jsonData, err := json.Marshal(email)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return &mcpSDK.ReadResourceResult{
		Contents: []*mcpSDK.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(jsonData),
			},
		},
	}, nil
}

// HandleSMSList handles the sms://list resource
// Supports query parameters: page, per_page
func (h *MCPHandlers) HandleSMSList(ctx context.Context, req *mcpSDK.ReadResourceRequest) (*mcpSDK.ReadResourceResult, error) {
	orgID := GetOrgID(ctx)
	if orgID == "" {
		return nil, fmt.Errorf("organization ID not found in context")
	}

	// Check quota
	if err := h.quotaChecker.CheckAndIncrement(ctx, orgID, datastore.OpSMSRead); err != nil {
		logger.FromContext(ctx).Warn("SMS read quota exceeded", zap.String("org_id", orgID))
		return nil, err
	}

	// Parse query parameters
	params, err := parseQueryParams(req.Params.URI)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URI: %w", err)
	}

	page := getIntQueryParam(params, "page", 1)
	perPage := getIntQueryParam(params, "per_page", 20)

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	// Get tenant
	tenant, err := h.tenantStore.GetTenant(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("tenant not found: %w", err)
	}

	// Query SMS messages using paginated method
	result, err := h.bqStore.ListSMSMessagesPaginated(ctx, tenant.BigQueryDataset, page, perPage)
	if err != nil {
		return nil, fmt.Errorf("failed to list SMS: %w", err)
	}

	jsonData, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return &mcpSDK.ReadResourceResult{
		Contents: []*mcpSDK.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(jsonData),
			},
		},
	}, nil
}

// HandleSMSDetails handles the sms://message/{messageSid} resource
func (h *MCPHandlers) HandleSMSDetails(ctx context.Context, req *mcpSDK.ReadResourceRequest) (*mcpSDK.ReadResourceResult, error) {
	orgID := GetOrgID(ctx)
	if orgID == "" {
		return nil, fmt.Errorf("organization ID not found in context")
	}

	// Check quota
	if err := h.quotaChecker.CheckAndIncrement(ctx, orgID, datastore.OpSMSRead); err != nil {
		logger.FromContext(ctx).Warn("SMS read quota exceeded", zap.String("org_id", orgID))
		return nil, err
	}

	// Extract message SID from URI (sms://message/{messageSid})
	parts := strings.Split(req.Params.URI, "/")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid URI format, expected sms://message/{messageSid}")
	}
	messageSID := parts[len(parts)-1]

	if messageSID == "" {
		return nil, fmt.Errorf("message SID is required")
	}

	// Get tenant
	tenant, err := h.tenantStore.GetTenant(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("tenant not found: %w", err)
	}

	// Get SMS details - for now, list recent SMS and filter
	// TODO: Add GetSMSByMessageSID method to BigQueryStore for better performance
	messages, err := h.bqStore.ListSMSMessages(ctx, tenant.BigQueryDataset, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to query SMS: %w", err)
	}

	// Find the specific SMS
	var sms *storage.SMSRecord
	for _, s := range messages {
		if s.MessageSID == messageSID {
			sms = s
			break
		}
	}

	if sms == nil {
		return nil, mcpSDK.ResourceNotFoundError(req.Params.URI)
	}

	jsonData, err := json.Marshal(sms)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return &mcpSDK.ReadResourceResult{
		Contents: []*mcpSDK.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(jsonData),
			},
		},
	}, nil
}

// ==================== TOOL HANDLERS (Write Operations) ====================

// SendEmailInput defines arguments for send_email tool
type SendEmailInput struct {
	AccountID string `json:"account_id"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

// SendEmailOutput defines the output for send_email tool
type SendEmailOutput struct {
	MessageID string `json:"message_id"`
	Success   bool   `json:"success"`
}

// SendEmail handles the send_email tool call
func (h *MCPHandlers) SendEmail(
	ctx context.Context,
	req *mcpSDK.CallToolRequest,
	input SendEmailInput,
) (*mcpSDK.CallToolResult, SendEmailOutput, error) {
	orgID := GetOrgID(ctx)
	if orgID == "" {
		return nil, SendEmailOutput{}, fmt.Errorf("organization ID not found in context")
	}

	userID := GetUserID(ctx)
	logger.FromContext(ctx).Info("Sending email via MCP",
		zap.String("org_id", orgID),
		zap.String("user_id", userID),
		zap.String("account_id", input.AccountID))

	// Check quota
	if err := h.quotaChecker.CheckAndIncrement(ctx, orgID, datastore.OpEmailSend); err != nil {
		logger.FromContext(ctx).Warn("Email send quota exceeded", zap.String("org_id", orgID))
		return nil, SendEmailOutput{}, err
	}

	// Validate input
	if input.AccountID == "" || input.To == "" || input.Subject == "" {
		return nil, SendEmailOutput{}, fmt.Errorf("account_id, to, and subject are required")
	}

	// Validate email format
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(input.To) {
		return nil, SendEmailOutput{}, fmt.Errorf("invalid email address format")
	}

	// Get tenant for namespace
	tenant, err := h.tenantStore.GetTenant(ctx, orgID)
	if err != nil {
		return nil, SendEmailOutput{}, fmt.Errorf("tenant not found: %w", err)
	}

	// Get Gmail account with decrypted tokens
	account, err := h.accountStore.GetAccountWithDecryptedTokens(ctx, tenant.Namespace, orgID, input.AccountID)
	if err != nil {
		return nil, SendEmailOutput{}, fmt.Errorf("account not found: %w", err)
	}

	// Verify account belongs to org
	if account.OrgID != orgID {
		return nil, SendEmailOutput{}, fmt.Errorf("account does not belong to organization")
	}

	// Token refresh callback
	callback := func(newAccessToken, newRefreshToken string, expiry time.Time) {
		if err := h.accountStore.UpdateTokens(ctx, tenant.Namespace, orgID, input.AccountID, newAccessToken, newRefreshToken, expiry); err != nil {
			logger.FromContext(ctx).Error("Failed to update tokens", zap.Error(err))
		}
	}

	// Get token expiry, default to zero time if nil
	expiry := time.Time{}
	if account.TokenExpiry != nil {
		expiry = *account.TokenExpiry
	}

	// Send email
	err = h.gmailClient.SendEmail(
		ctx,
		account.AccessToken,
		account.RefreshToken,
		input.To,
		input.Subject,
		input.Body,
		expiry,
		callback,
	)
	if err != nil {
		return nil, SendEmailOutput{}, fmt.Errorf("failed to send email: %w", err)
	}

	logger.FromContext(ctx).Info("Email sent successfully",
		zap.String("account_id", input.AccountID),
		zap.String("to", input.To))

	return nil, SendEmailOutput{
		MessageID: input.AccountID, // Note: Gmail API doesn't return message ID immediately from send
		Success:   true,
	}, nil
}

// SendSMSInput defines arguments for send_sms tool
type SendSMSInput struct {
	To   string `json:"to"`
	Body string `json:"body"`
}

// SendSMSOutput defines the output for send_sms tool
type SendSMSOutput struct {
	MessageSID string `json:"message_sid"`
	Success    bool   `json:"success"`
}

// SendSMS handles the send_sms tool call
func (h *MCPHandlers) SendSMS(
	ctx context.Context,
	req *mcpSDK.CallToolRequest,
	input SendSMSInput,
) (*mcpSDK.CallToolResult, SendSMSOutput, error) {
	orgID := GetOrgID(ctx)
	if orgID == "" {
		return nil, SendSMSOutput{}, fmt.Errorf("organization ID not found in context")
	}

	userID := GetUserID(ctx)
	logger.FromContext(ctx).Info("Sending SMS via MCP",
		zap.String("org_id", orgID),
		zap.String("user_id", userID),
		zap.String("to", input.To))

	// Check quota
	if err := h.quotaChecker.CheckAndIncrement(ctx, orgID, datastore.OpSMSSend); err != nil {
		logger.FromContext(ctx).Warn("SMS send quota exceeded", zap.String("org_id", orgID))
		return nil, SendSMSOutput{}, err
	}

	// Validate input
	if input.To == "" || input.Body == "" {
		return nil, SendSMSOutput{}, fmt.Errorf("to and body are required")
	}

	// Validate phone number format (E.164)
	phoneRegex := regexp.MustCompile(`^\+[1-9]\d{1,14}$`)
	if !phoneRegex.MatchString(input.To) {
		return nil, SendSMSOutput{}, fmt.Errorf("invalid phone number format, must be E.164 format (e.g., +1234567890)")
	}

	// Get tenant for BigQuery dataset
	tenant, err := h.tenantStore.GetTenant(ctx, orgID)
	if err != nil {
		return nil, SendSMSOutput{}, fmt.Errorf("tenant not found: %w", err)
	}

	// Send SMS
	sms, err := h.twilioClient.SendSMS(ctx, input.To, input.Body, orgID)
	if err != nil {
		return nil, SendSMSOutput{}, fmt.Errorf("failed to send SMS: %w", err)
	}

	// Store outbound message in BigQuery
	if err := h.bqStore.InsertSMSMessage(ctx, tenant.BigQueryDataset, sms, orgID); err != nil {
		logger.FromContext(ctx).Warn("Failed to store SMS in BigQuery",
			zap.String("message_sid", sms.MessageSID),
			zap.Error(err))
		// Don't fail the request - SMS was sent successfully
	}

	logger.FromContext(ctx).Info("SMS sent successfully",
		zap.String("message_sid", sms.MessageSID),
		zap.String("to", input.To))

	return nil, SendSMSOutput{
		MessageSID: sms.MessageSID,
		Success:    true,
	}, nil
}

// ==================== Helper Functions ====================

// Helper function to parse query parameters from URI
func parseQueryParams(uri string) (url.Values, error) {
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("invalid URI: %w", err)
	}
	return parsedURL.Query(), nil
}

// Helper function to parse int query parameter
func getIntQueryParam(params url.Values, key string, defaultValue int) int {
	if val := params.Get(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultValue
}
