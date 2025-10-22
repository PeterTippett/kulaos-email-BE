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
			gmail.GmailModifyScope,
			gmail.GmailSendScope,
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
	fmt.Printf("🔄 [OAUTH] Attempting to refresh token...\n")

	token := &oauth2.Token{
		RefreshToken: refreshToken,
	}

	tokenSource := g.config.TokenSource(ctx, token)
	newToken, err := tokenSource.Token()
	if err != nil {
		fmt.Printf("❌ [OAUTH] Token refresh failed: %v\n", err)
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	fmt.Printf("✅ [OAUTH] Token refresh successful - new expiry: %v\n", newToken.Expiry)
	return newToken, nil
}

func (g *GmailOAuth) GetClient(ctx context.Context, accessToken, refreshToken string) *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}
}

// GetTokenSource creates a TokenSource that automatically handles token refresh
func (g *GmailOAuth) GetTokenSource(ctx context.Context, accessToken, refreshToken string) oauth2.TokenSource {
	token := &oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}
	return g.config.TokenSource(ctx, token)
}

// TokenRefreshCallback is called when tokens are refreshed
type TokenRefreshCallback func(newAccessToken, newRefreshToken string, expiry time.Time)

// GetTokenSourceWithCallback creates a TokenSource that calls a callback when tokens are refreshed
func (g *GmailOAuth) GetTokenSourceWithCallback(ctx context.Context, accessToken, refreshToken string, callback TokenRefreshCallback) oauth2.TokenSource {
	token := &oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		// Don't set Expiry here - let the OAuth2 library handle it
	}

	baseTokenSource := g.config.TokenSource(ctx, token)

	// Wrap the TokenSource to detect when tokens are refreshed
	return oauth2.ReuseTokenSource(token, &callbackTokenSource{
		base:     baseTokenSource,
		callback: callback,
	})
}

// GetTokenSourceWithCallbackAndExpiry creates a TokenSource with explicit expiry that calls a callback when tokens are refreshed
func (g *GmailOAuth) GetTokenSourceWithCallbackAndExpiry(ctx context.Context, accessToken, refreshToken string, expiry time.Time, callback TokenRefreshCallback) oauth2.TokenSource {
	token := &oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expiry:       expiry,
	}

	baseTokenSource := g.config.TokenSource(ctx, token)

	// Wrap the TokenSource to detect when tokens are refreshed
	return oauth2.ReuseTokenSource(token, &callbackTokenSource{
		base:     baseTokenSource,
		callback: callback,
	})
}

// callbackTokenSource wraps a TokenSource to detect token refreshes
type callbackTokenSource struct {
	base            oauth2.TokenSource
	callback        TokenRefreshCallback
	lastAccessToken string
	lastExpiry      time.Time
}

func (c *callbackTokenSource) Token() (*oauth2.Token, error) {
	token, err := c.base.Token()
	if err != nil {
		fmt.Printf("❌ [OAUTH] TokenSource.Token() failed: %v\n", err)
		return nil, err
	}

	// Check if token has been refreshed by comparing access token or expiry
	tokenRefreshed := false
	if c.lastAccessToken != "" && c.lastAccessToken != token.AccessToken {
		fmt.Printf("🔄 [OAUTH] Token refreshed - access token changed\n")
		tokenRefreshed = true
	} else if !c.lastExpiry.IsZero() && !token.Expiry.IsZero() && !token.Expiry.Equal(c.lastExpiry) {
		fmt.Printf("🔄 [OAUTH] Token refreshed - expiry changed from %v to %v\n", c.lastExpiry, token.Expiry)
		tokenRefreshed = true
	}

	// If callback is provided and token has been refreshed, call the callback
	if c.callback != nil && tokenRefreshed {
		fmt.Printf("🔄 [OAUTH] Calling token refresh callback\n")
		c.callback(token.AccessToken, token.RefreshToken, token.Expiry)
	}

	// Update our tracking variables
	c.lastAccessToken = token.AccessToken
	c.lastExpiry = token.Expiry

	return token, nil
}

// IsTokenExpired checks if a token is expired or will expire soon
func (g *GmailOAuth) IsTokenExpired(tokenExpiry time.Time) bool {
	// Consider token expired if it expires within the next 5 minutes
	isExpired := time.Now().Add(5 * time.Minute).After(tokenExpiry)
	if isExpired {
		fmt.Printf("⚠️ [OAUTH] Token is expired or expires within 5 minutes (expiry: %v)\n", tokenExpiry)
	}
	return isExpired
}

// ShouldRefreshToken checks if a token should be refreshed proactively
func (g *GmailOAuth) ShouldRefreshToken(tokenExpiry time.Time) bool {
	// Refresh token if it expires within the next 10 minutes
	shouldRefresh := time.Now().Add(10 * time.Minute).After(tokenExpiry)
	if shouldRefresh {
		fmt.Printf("🔄 [OAUTH] Token should be refreshed proactively (expires within 10 minutes: %v)\n", tokenExpiry)
	}
	return shouldRefresh
}

// RefreshTokenWithRetry attempts to refresh a token with retry logic
func (g *GmailOAuth) RefreshTokenWithRetry(ctx context.Context, refreshToken string, maxRetries int) (*oauth2.Token, error) {
	fmt.Printf("🔄 [OAUTH] Starting token refresh with retry (max %d attempts)\n", maxRetries)
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		fmt.Printf("🔄 [OAUTH] Token refresh attempt %d/%d\n", i+1, maxRetries)
		token, err := g.RefreshToken(ctx, refreshToken)
		if err == nil {
			fmt.Printf("✅ [OAUTH] Token refresh succeeded on attempt %d\n", i+1)
			return token, nil
		}

		lastErr = err
		fmt.Printf("❌ [OAUTH] Token refresh attempt %d failed: %v\n", i+1, err)

		// If this is not the last retry, wait before trying again
		if i < maxRetries-1 {
			// Exponential backoff: wait 1s, 2s, 4s, etc.
			waitTime := time.Duration(1<<uint(i)) * time.Second
			fmt.Printf("⏳ [OAUTH] Waiting %v before retry...\n", waitTime)
			select {
			case <-ctx.Done():
				fmt.Printf("❌ [OAUTH] Context cancelled during retry wait\n")
				return nil, ctx.Err()
			case <-time.After(waitTime):
				// Continue to next retry
			}
		}
	}

	fmt.Printf("❌ [OAUTH] Token refresh failed after %d attempts: %v\n", maxRetries, lastErr)
	return nil, fmt.Errorf("failed to refresh token after %d attempts: %w", maxRetries, lastErr)
}
