package types

import "time"

// EmailMessage represents a parsed email message
type EmailMessage struct {
	MessageID     string
	ThreadID      string
	Sender        string
	Recipients    []string // To recipients
	CCRecipients  []string // CC recipients
	BCCRecipients []string // BCC recipients (if available)
	Subject       string
	BodyText      string
	BodyHTML      string
	BodyMarkdown  string
	ReceivedAt    time.Time
	Labels        []string
	IsRead        bool
	InternalDate  int64
}
