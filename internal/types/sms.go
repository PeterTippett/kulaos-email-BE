package types

import "time"

// SMSMessage represents an SMS message (inbound or outbound)
type SMSMessage struct {
	MessageSID string    // Twilio message SID
	From       string    // Phone number (e.g., +1234567890)
	To         string    // Phone number (e.g., +1234567890)
	Body       string    // SMS content
	Direction  string    // "inbound" or "outbound"
	Status     string    // Twilio status (queued, sent, delivered, failed, etc.)
	ReceivedAt time.Time // When message was received (for inbound)
	SentAt     time.Time // When message was sent (for outbound)
	AccountID  string    // Tenant account ID for routing
}
