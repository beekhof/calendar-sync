package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/oauth2"
)

// TokenStore is an interface for saving and loading OAuth tokens.
type TokenStore interface {
	SaveToken(token *oauth2.Token) error
	LoadToken() (*oauth2.Token, error)
}

// autoSaveTokenSource wraps an oauth2.TokenSource and automatically saves refreshed tokens.
type autoSaveTokenSource struct {
	source    oauth2.TokenSource
	tokenStore TokenStore
	lastToken *oauth2.Token
}

// Token implements oauth2.TokenSource and saves the token if it was refreshed.
func (a *autoSaveTokenSource) Token() (*oauth2.Token, error) {
	token, err := a.source.Token()
	if err != nil {
		return nil, err
	}

	// Check if the token was refreshed by comparing access tokens
	if a.lastToken == nil || a.lastToken.AccessToken != token.AccessToken {
		// Token was refreshed, save it
		if err := a.tokenStore.SaveToken(token); err != nil {
			return nil, fmt.Errorf("failed to save refreshed token: %w", err)
		}
		a.lastToken = token
	}

	return token, nil
}

// GetAuthenticatedClient returns an authenticated HTTP client using OAuth 2.0.
// If no token exists, it will guide the user through the interactive OAuth flow.
func GetAuthenticatedClient(ctx context.Context, oauthConfig *oauth2.Config, tokenStore TokenStore) (*http.Client, error) {
	// Attempt to load an existing token
	token, err := tokenStore.LoadToken()
	if err != nil {
		return nil, fmt.Errorf("failed to load token: %w", err)
	}

	// If token is nil (first run), perform interactive OAuth flow
	if token == nil {
		// Generate auth URL
		authURL := oauthConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
		
		fmt.Println("Please visit the following URL to authorize the application:")
		fmt.Println(authURL)
		fmt.Print("Enter the authorization code: ")

		// Read the auth code from stdin
		var code string
		if _, err := fmt.Scanln(&code); err != nil {
			return nil, fmt.Errorf("failed to read authorization code: %w", err)
		}

		// Exchange the code for a token
		token, err = oauthConfig.Exchange(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("failed to exchange authorization code: %w", err)
		}

		// Save the new token
		if err := tokenStore.SaveToken(token); err != nil {
			return nil, fmt.Errorf("failed to save token: %w", err)
		}
	}

	// Create a token source
	tokenSource := oauthConfig.TokenSource(ctx, token)

	// Wrap the token source to auto-save refreshed tokens
	autoSaveSource := &autoSaveTokenSource{
		source:     oauth2.ReuseTokenSource(token, tokenSource),
		tokenStore: tokenStore,
		lastToken:  token,
	}

	// Return a new HTTP client using the auto-save token source
	return oauth2.NewClient(ctx, autoSaveSource), nil
}

// GetAuthenticatedClientWithReader is a helper function for testing that allows
// injecting a custom reader for the authorization code.
func GetAuthenticatedClientWithReader(ctx context.Context, oauthConfig *oauth2.Config, tokenStore TokenStore, reader io.Reader) (*http.Client, error) {
	// Attempt to load an existing token
	token, err := tokenStore.LoadToken()
	if err != nil {
		return nil, fmt.Errorf("failed to load token: %w", err)
	}

	// If token is nil (first run), perform interactive OAuth flow
	if token == nil {
		// Generate auth URL
		authURL := oauthConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
		
		fmt.Println("Please visit the following URL to authorize the application:")
		fmt.Println(authURL)
		fmt.Print("Enter the authorization code: ")

		// Read the auth code from the provided reader
		var code string
		if _, err := fmt.Fscanln(reader, &code); err != nil {
			return nil, fmt.Errorf("failed to read authorization code: %w", err)
		}

		// Exchange the code for a token
		token, err = oauthConfig.Exchange(ctx, code)
		if err != nil {
			return nil, fmt.Errorf("failed to exchange authorization code: %w", err)
		}

		// Save the new token
		if err := tokenStore.SaveToken(token); err != nil {
			return nil, fmt.Errorf("failed to save token: %w", err)
		}
	}

	// Create a token source
	tokenSource := oauthConfig.TokenSource(ctx, token)

	// Wrap the token source to auto-save refreshed tokens
	autoSaveSource := &autoSaveTokenSource{
		source:     oauth2.ReuseTokenSource(token, tokenSource),
		tokenStore: tokenStore,
		lastToken:  token,
	}

	// Return a new HTTP client using the auto-save token source
	return oauth2.NewClient(ctx, autoSaveSource), nil
}

