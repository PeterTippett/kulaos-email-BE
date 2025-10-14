package types

import "time"

// EmailMessage represents a parsed email message
type EmailMessage struct {
	MessageID    string
	ThreadID     string
	Sender       string
	Subject      string
	BodyText     string
	BodyHTML     string
	BodyMarkdown string
	ReceivedAt   time.Time
	Labels       []string
	IsRead       bool
	InternalDate int64
}
