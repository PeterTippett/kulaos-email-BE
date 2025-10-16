package gmail

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/yourusername/email-service/internal/gmail/parser"
	"github.com/yourusername/email-service/internal/types"
)

// hasLabel returns true if the given label is present in the slice
func hasLabel(labels []string, target string) bool {
	for _, l := range labels {
		if l == target {
			return true
		}
	}
	return false
}

// isMessageRelevant returns true if the message should be processed
// Only process messages that are in INBOX or SENT and are NOT drafts
func isMessageRelevant(labels []string) bool {
	// Must have INBOX or SENT label
	hasInboxOrSent := hasLabel(labels, "INBOX") || hasLabel(labels, "SENT")
	// Must NOT be a draft
	notDraft := !hasLabel(labels, "DRAFT")

	return hasInboxOrSent && notDraft
}

type GmailClient struct {
	oauth *GmailOAuth
}

func NewGmailClient(oauth *GmailOAuth) *GmailClient {
	return &GmailClient{oauth: oauth}
}

type WatchResponse struct {
	ChannelID      string
	Expiration     time.Time
	StartHistoryID int64
}

// SetupWatch sets up Gmail push notifications
func (g *GmailClient) SetupWatch(ctx context.Context, accessToken, refreshToken, topicName string) (*WatchResponse, error) {
	fmt.Printf("🔄 [GMAIL] Setting up watch with TokenSource for topic: %s\n", topicName)
	// Build a TokenSource from both tokens so oauth2 handles refreshing properly
	ts := g.oauth.GetTokenSource(ctx, accessToken, refreshToken)
	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("failed to create gmail service: %w", err)
	}

	watchRequest := &gmail.WatchRequest{
		TopicName:           topicName,
		LabelIds:            []string{"INBOX", "SENT"},
		LabelFilterBehavior: "include",
	}

	watchResp, err := gmailService.Users.Watch("me", watchRequest).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to setup watch: %w", err)
	}

	expirationMs := watchResp.Expiration
	expiration := time.Unix(0, expirationMs*int64(time.Millisecond))

	return &WatchResponse{
		ChannelID:      fmt.Sprintf("%d", watchResp.HistoryId),
		Expiration:     expiration,
		StartHistoryID: int64(watchResp.HistoryId),
	}, nil
}

// FetchMessage fetches a single message by ID
func (g *GmailClient) FetchMessage(ctx context.Context, accessToken, refreshToken, messageID string) (*types.EmailMessage, error) {
	fmt.Printf("🔄 [GMAIL] Fetching message %s with TokenSource\n", messageID)
	ts := g.oauth.GetTokenSource(ctx, accessToken, refreshToken)
	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("failed to create gmail service: %w", err)
	}

	// Request labels explicitly to ensure we can filter correctly
	msg, err := gmailService.Users.Messages.Get("me", messageID).Format("full").Do()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch message: %w", err)
	}

	return parser.ParseMessage(msg)
}

// ListMessages lists recent messages
func (g *GmailClient) ListMessages(ctx context.Context, accessToken, refreshToken string, maxResults int64) ([]*types.EmailMessage, error) {
	ts := g.oauth.GetTokenSource(ctx, accessToken, refreshToken)
	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("failed to create gmail service: %w", err)
	}

	listResp, err := gmailService.Users.Messages.List("me").MaxResults(maxResults).LabelIds("INBOX", "SENT").Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}

	var messages []*types.EmailMessage
	for _, msgRef := range listResp.Messages {
		msg, err := g.FetchMessage(ctx, accessToken, refreshToken, msgRef.Id)
		if err != nil {
			continue
		}
		// Filter out drafts and only keep INBOX or SENT
		fmt.Printf("🔍 [GMAIL] Message %s labels: %v\n", msg.MessageID, msg.Labels)
		if isMessageRelevant(msg.Labels) {
			fmt.Printf("✅ [GMAIL] Message %s passed filter (INBOX/SENT, not DRAFT)\n", msg.MessageID)
			messages = append(messages, msg)
		} else {
			fmt.Printf("❌ [GMAIL] Message %s filtered out (not INBOX/SENT or is DRAFT)\n", msg.MessageID)
		}
	}

	return messages, nil
}

// FetchNewMessages fetches messages since a given history ID (legacy, single page)
func (g *GmailClient) FetchNewMessages(ctx context.Context, accessToken, refreshToken string, historyID uint64) ([]*types.EmailMessage, error) {
	ts := g.oauth.GetTokenSource(ctx, accessToken, refreshToken)
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
			// Filter out drafts and only keep INBOX or SENT
			fmt.Printf("🔍 [GMAIL] Message %s labels: %v\n", fullMsg.MessageID, fullMsg.Labels)
			if isMessageRelevant(fullMsg.Labels) {
				fmt.Printf("✅ [GMAIL] Message %s passed filter (INBOX/SENT, not DRAFT)\n", fullMsg.MessageID)
				messages = append(messages, fullMsg)
			} else {
				fmt.Printf("❌ [GMAIL] Message %s filtered out (not INBOX/SENT or is DRAFT)\n", fullMsg.MessageID)
			}
		}
	}

	return messages, nil
}

// FetchNewMessagesPaged fetches messages since a given history ID, following pages and returning the latest history ID seen
func (g *GmailClient) FetchNewMessagesPaged(ctx context.Context, accessToken, refreshToken string, startHistoryID int64) ([]*types.EmailMessage, int64, error) {
	fmt.Printf("🔄 [GMAIL] Fetching new messages (paged) with TokenSource from history ID: %d\n", startHistoryID)
	ts := g.oauth.GetTokenSource(ctx, accessToken, refreshToken)
	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create gmail service: %w", err)
	}

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

		for _, h := range history.History {
			if int64(h.Id) > latestHistoryID {
				latestHistoryID = int64(h.Id)
			}
			for _, msg := range h.MessagesAdded {
				fullMsg, err := g.FetchMessage(ctx, accessToken, refreshToken, msg.Message.Id)
				if err != nil {
					continue
				}
				// Filter out drafts and only keep INBOX or SENT
				fmt.Printf("🔍 [GMAIL] Message %s labels: %v\n", fullMsg.MessageID, fullMsg.Labels)
				if isMessageRelevant(fullMsg.Labels) {
					fmt.Printf("✅ [GMAIL] Message %s passed filter (INBOX/SENT, not DRAFT)\n", fullMsg.MessageID)
					allMessages = append(allMessages, fullMsg)
				} else {
					fmt.Printf("❌ [GMAIL] Message %s filtered out (not INBOX/SENT or is DRAFT)\n", fullMsg.MessageID)
				}
			}
		}

		if history.NextPageToken == "" {
			break
		}
		pageToken = history.NextPageToken
	}

	return allMessages, latestHistoryID, nil
}

// StopWatch stops Gmail push notifications for an account
func (g *GmailClient) StopWatch(ctx context.Context, accessToken, refreshToken string) error {
	ts := g.oauth.GetTokenSource(ctx, accessToken, refreshToken)
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
