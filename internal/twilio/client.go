package twilio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yourusername/email-service/internal/types"
)

// TwilioClient handles SMS sending via Twilio API
type TwilioClient struct {
	accountSID     string
	authToken      string
	phoneNumber    string
	backendBaseURL string
	httpClient     *http.Client
}

// NewTwilioClient creates a new Twilio client instance
func NewTwilioClient(accountSID, authToken, phoneNumber, backendBaseURL string) *TwilioClient {
	return &TwilioClient{
		accountSID:     accountSID,
		authToken:      authToken,
		phoneNumber:    phoneNumber,
		backendBaseURL: backendBaseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// twilioResponse represents the Twilio API response for message creation
type twilioResponse struct {
	SID          string  `json:"sid"`
	DateCreated  string  `json:"date_created"`
	DateUpdated  string  `json:"date_updated"`
	DateSent     *string `json:"date_sent"`
	AccountSID   string  `json:"account_sid"`
	To           string  `json:"to"`
	From         string  `json:"from"`
	Body         string  `json:"body"`
	Status       string  `json:"status"`
	Direction    string  `json:"direction"`
	ErrorCode    *int    `json:"error_code"`
	ErrorMessage *string `json:"error_message"`
}

// SendSMS sends an SMS message via Twilio API
func (t *TwilioClient) SendSMS(ctx context.Context, to, body, orgID string) (*types.SMSMessage, error) {
	// Build Twilio API URL
	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", t.accountSID)

	// Prepare form data
	data := url.Values{}
	data.Set("To", to)
	data.Set("From", t.phoneNumber)
	data.Set("Body", body)

	// Set status callback URL with org_id if backend URL is configured
	if t.backendBaseURL != "" && orgID != "" {
		statusCallbackURL := fmt.Sprintf("%s/webhooks/%s/twilio/status", t.backendBaseURL, orgID)
		data.Set("StatusCallback", statusCallbackURL)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(t.accountSID, t.authToken)

	// Send request
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twilio API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse response
	var twilioResp twilioResponse
	if err := json.Unmarshal(bodyBytes, &twilioResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for Twilio-level errors
	if twilioResp.ErrorCode != nil && *twilioResp.ErrorCode != 0 {
		errorMsg := "unknown error"
		if twilioResp.ErrorMessage != nil {
			errorMsg = *twilioResp.ErrorMessage
		}
		return nil, fmt.Errorf("twilio error %d: %s", *twilioResp.ErrorCode, errorMsg)
	}

	// Convert to SMSMessage
	message := &types.SMSMessage{
		MessageSID: twilioResp.SID,
		From:       twilioResp.From,
		To:         twilioResp.To,
		Body:       twilioResp.Body,
		Direction:  "outbound",
		Status:     twilioResp.Status,
		SentAt:     time.Now(),
		ReceivedAt: time.Time{}, // Not applicable for outbound
	}

	return message, nil
}
