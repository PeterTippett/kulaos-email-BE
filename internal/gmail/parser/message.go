package parser

import (
	"encoding/base64"
	"strings"
	"time"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"google.golang.org/api/gmail/v1"

	"github.com/yourusername/email-service/internal/types"
)

// ParseMessage parses a Gmail message into our EmailMessage type
func ParseMessage(msg *gmail.Message) (*types.EmailMessage, error) {
	emailMsg := &types.EmailMessage{
		MessageID:    msg.Id,
		ThreadID:     msg.ThreadId,
		Labels:       msg.LabelIds,
		InternalDate: msg.InternalDate,
	}

	// Parse headers
	for _, header := range msg.Payload.Headers {
		switch header.Name {
		case "From":
			emailMsg.Sender = header.Value
		case "To":
			emailMsg.Recipients = parseEmailList(header.Value)
		case "Cc":
			emailMsg.CCRecipients = parseEmailList(header.Value)
		case "Bcc":
			emailMsg.BCCRecipients = parseEmailList(header.Value)
		case "Subject":
			emailMsg.Subject = header.Value
		case "Date":
			if t, err := time.Parse(time.RFC1123Z, header.Value); err == nil {
				emailMsg.ReceivedAt = t
			}
		}
	}

	// If ReceivedAt is not set from Date header, use InternalDate
	if emailMsg.ReceivedAt.IsZero() {
		emailMsg.ReceivedAt = time.Unix(0, msg.InternalDate*int64(time.Millisecond))
	}

	// Parse body
	emailMsg.BodyText, emailMsg.BodyHTML = ParseBody(msg.Payload)

	// Convert HTML to Markdown if HTML content exists
	if emailMsg.BodyHTML != "" {
		emailMsg.BodyMarkdown = ConvertHTMLToMarkdown(emailMsg.BodyHTML)
	}

	// Check if read
	emailMsg.IsRead = true
	for _, label := range msg.LabelIds {
		if label == "UNREAD" {
			emailMsg.IsRead = false
			break
		}
	}

	return emailMsg, nil
}

// parseEmailList parses a comma-separated list of email addresses
// Handles formats like "Name <email@example.com>, email2@example.com"
func parseEmailList(emailList string) []string {
	if emailList == "" {
		return []string{}
	}

	var emails []string
	// Split by comma and clean up each email
	for _, email := range strings.Split(emailList, ",") {
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}

		// Extract email from "Name <email@example.com>" format
		if idx := strings.Index(email, "<"); idx != -1 {
			if endIdx := strings.Index(email[idx:], ">"); endIdx != -1 {
				email = strings.TrimSpace(email[idx+1 : idx+endIdx])
			}
		}

		emails = append(emails, email)
	}

	return emails
}

// ParseBody extracts text and HTML content from message payload
func ParseBody(payload *gmail.MessagePart) (text, html string) {
	if payload.Body != nil && payload.Body.Data != "" {
		decoded, _ := base64.URLEncoding.DecodeString(payload.Body.Data)
		if payload.MimeType == "text/plain" {
			text = string(decoded)
		} else if payload.MimeType == "text/html" {
			html = string(decoded)
		}
	}

	for _, part := range payload.Parts {
		if part.MimeType == "text/plain" && part.Body != nil && part.Body.Data != "" {
			decoded, _ := base64.URLEncoding.DecodeString(part.Body.Data)
			text = string(decoded)
		} else if part.MimeType == "text/html" && part.Body != nil && part.Body.Data != "" {
			decoded, _ := base64.URLEncoding.DecodeString(part.Body.Data)
			html = string(decoded)
		}

		if len(part.Parts) > 0 {
			t, h := ParseBody(part)
			if text == "" {
				text = t
			}
			if html == "" {
				html = h
			}
		}
	}

	return
}

// ConvertHTMLToMarkdown converts HTML content to Markdown
func ConvertHTMLToMarkdown(html string) string {
	if html == "" {
		return ""
	}

	options := &md.Options{
		HeadingStyle:       "atx",
		BulletListMarker:   "-",
		CodeBlockStyle:     "fenced",
		Fence:              "```",
		EmDelimiter:        "*",
		StrongDelimiter:    "**",
		LinkStyle:          "inlined",
		LinkReferenceStyle: "full",
	}
	converter := md.NewConverter("", true, options)

	markdown, err := converter.ConvertString(html)
	if err != nil {
		// Fallback to plain text if Markdown conversion fails
		return strings.TrimSpace(html)
	}

	return strings.TrimSpace(markdown)
}
