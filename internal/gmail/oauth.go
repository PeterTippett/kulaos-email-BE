package gmail

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type GmailOAuth struct {
	config *oauth2.Config
}

func NewGmailOAuth(clientID, clientSecret, redirectURI string) *GmailOAuth {
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURI,
		Scopes: []string{
			gmail.GmailReadonlyScope,
		},
		Endpoint: google.Endpoint,
	}

	return &GmailOAuth{config: config}
}

func (g *GmailOAuth) GetAuthURL(state string) string {
	return g.config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

type TokenResponse struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	Email        string
}

func (g *GmailOAuth) ExchangeCode(ctx context.Context, code string) (*TokenResponse, error) {
	token, err := g.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	// Get user email address
	client := g.config.Client(ctx, token)
	gmailService, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("failed to create gmail service: %w", err)
	}

	profile, err := gmailService.Users.GetProfile("me").Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get profile: %w", err)
	}

	return &TokenResponse{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
		Email:        profile.EmailAddress,
	}, nil
}

func (g *GmailOAuth) RefreshToken(ctx context.Context, refreshToken string) (*oauth2.Token, error) {
	token := &oauth2.Token{
		RefreshToken: refreshToken,
	}

	tokenSource := g.config.TokenSource(ctx, token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	return newToken, nil
}

func (g *GmailOAuth) GetClient(ctx context.Context, accessToken, refreshToken string) *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}
}
