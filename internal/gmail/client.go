package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/yourusername/email-service/internal/types"
)

type GmailClient struct {
	oauth *GmailOAuth
}

func NewGmailClient(oauth *GmailOAuth) *GmailClient {
	return &GmailClient{oauth: oauth}
}

type WatchResponse struct {
	ChannelID  string
	Expiration time.Time
}

// SetupWatch sets up Gmail push notifications
func (g *GmailClient) SetupWatch(ctx context.Context, accessToken, refreshToken, topicName string) (*WatchResponse, error) {
	// Build a TokenSource from the refresh token so oauth2 handles refreshing
	ts := g.oauth.config.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("failed to create gmail service: %w", err)
	}

	watchRequest := &gmail.WatchRequest{
		TopicName: topicName,
		LabelIds:  []string{"INBOX"},
	}

	watchResp, err := gmailService.Users.Watch("me", watchRequest).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to setup watch: %w", err)
	}

	expirationMs := watchResp.Expiration
	expiration := time.Unix(0, expirationMs*int64(time.Millisecond))

	return &WatchResponse{
		ChannelID:  fmt.Sprintf("%d", watchResp.HistoryId),
		Expiration: expiration,
	}, nil
}

// FetchMessage fetches a single message by ID
func (g *GmailClient) FetchMessage(ctx context.Context, accessToken, refreshToken, messageID string) (*types.EmailMessage, error) {
	ts := g.oauth.config.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("failed to create gmail service: %w", err)
	}

	msg, err := gmailService.Users.Messages.Get("me", messageID).Format("full").Do()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch message: %w", err)
	}

	return g.parseMessage(msg)
}

// ListMessages lists recent messages
func (g *GmailClient) ListMessages(ctx context.Context, accessToken, refreshToken string, maxResults int64) ([]*types.EmailMessage, error) {
	ts := g.oauth.config.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("failed to create gmail service: %w", err)
	}

	listResp, err := gmailService.Users.Messages.List("me").MaxResults(maxResults).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}

	var messages []*types.EmailMessage
	for _, msgRef := range listResp.Messages {
		msg, err := g.FetchMessage(ctx, accessToken, refreshToken, msgRef.Id)
		if err != nil {
			continue
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

// FetchNewMessages fetches messages since a given history ID
func (g *GmailClient) FetchNewMessages(ctx context.Context, accessToken, refreshToken string, historyID uint64) ([]*types.EmailMessage, error) {
	ts := g.oauth.config.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("failed to create gmail service: %w", err)
	}

	history, err := gmailService.Users.History.List("me").StartHistoryId(historyID).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch history: %w", err)
	}

	var messages []*types.EmailMessage

	for _, h := range history.History {
		for _, msg := range h.MessagesAdded {
			fullMsg, err := g.FetchMessage(ctx, accessToken, refreshToken, msg.Message.Id)
			if err != nil {
				continue
			}
			messages = append(messages, fullMsg)
		}
	}

	return messages, nil
}

// StopWatch stops Gmail push notifications for an account
func (g *GmailClient) StopWatch(ctx context.Context, accessToken, refreshToken string) error {
	ts := g.oauth.config.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("failed to create gmail service: %w", err)
	}

	err = gmailService.Users.Stop("me").Do()
	if err != nil {
		return fmt.Errorf("failed to stop watch: %w", err)
	}

	return nil
}

func (g *GmailClient) parseMessage(msg *gmail.Message) (*types.EmailMessage, error) {
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
	emailMsg.BodyText, emailMsg.BodyHTML = g.parseBody(msg.Payload)

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

func (g *GmailClient) parseBody(payload *gmail.MessagePart) (text, html string) {
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
			t, h := g.parseBody(part)
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
