package main

import (
	"context"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// mockTokenStore is a mock implementation of TokenStore for testing.
type mockTokenStore struct {
	token *oauth2.Token
	savedTokens []*oauth2.Token
}

func (m *mockTokenStore) SaveToken(token *oauth2.Token) error {
	m.savedTokens = append(m.savedTokens, token)
	m.token = token
	return nil
}

func (m *mockTokenStore) LoadToken() (*oauth2.Token, error) {
	return m.token, nil
}

func TestGetAuthenticatedClient_TokenExists(t *testing.T) {
	ctx := context.Background()

	// Create a mock token store with a valid, non-expired token
	expiry := time.Now().Add(1 * time.Hour)
	mockStore := &mockTokenStore{
		token: &oauth2.Token{
			AccessToken:  "test-access-token",
			RefreshToken: "test-refresh-token",
			Expiry:       expiry,
			TokenType:    "Bearer",
		},
	}

	// Create a minimal OAuth config (won't be used since token exists)
	oauthConfig := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
		Scopes:       []string{"https://www.googleapis.com/auth/calendar"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
	}

	// Get authenticated client
	client, err := GetAuthenticatedClient(ctx, oauthConfig, mockStore)
	if err != nil {
		t.Fatalf("GetAuthenticatedClient() returned an error: %v", err)
	}

	if client == nil {
		t.Fatal("GetAuthenticatedClient() returned nil client")
	}

	// Verify it's not nil (it should be an *http.Client)
	_ = client
}

