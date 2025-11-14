package main

import (
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestFileTokenStore_SaveLoad(t *testing.T) {
	// Create a temporary directory for the token file
	tempDir := t.TempDir()
	tokenPath := tempDir + "/token.json"

	// Create a new token store
	store := NewFileTokenStore(tokenPath)

	// Create a sample token
	expiry := time.Now().Add(1 * time.Hour)
	token := &oauth2.Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		Expiry:       expiry,
		TokenType:    "Bearer",
	}

	// Save the token
	if err := store.SaveToken(token); err != nil {
		t.Fatalf("SaveToken() returned an error: %v", err)
	}

	// Load the token
	loadedToken, err := store.LoadToken()
	if err != nil {
		t.Fatalf("LoadToken() returned an error: %v", err)
	}

	if loadedToken == nil {
		t.Fatal("LoadToken() returned nil token")
	}

	// Verify the loaded token matches the saved token
	if loadedToken.AccessToken != token.AccessToken {
		t.Errorf("Expected AccessToken to be '%s', got '%s'", token.AccessToken, loadedToken.AccessToken)
	}

	if loadedToken.RefreshToken != token.RefreshToken {
		t.Errorf("Expected RefreshToken to be '%s', got '%s'", token.RefreshToken, loadedToken.RefreshToken)
	}

	if !loadedToken.Expiry.Equal(token.Expiry) {
		t.Errorf("Expected Expiry to be %v, got %v", token.Expiry, loadedToken.Expiry)
	}
}

func TestFileTokenStore_LoadEmpty(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()
	tokenPath := tempDir + "/nonexistent.json"

	// Create a new token store pointing to a non-existent file
	store := NewFileTokenStore(tokenPath)

	// Try to load the token
	token, err := store.LoadToken()
	if err != nil {
		t.Fatalf("LoadToken() should not return an error for non-existent file, got: %v", err)
	}

	if token != nil {
		t.Errorf("LoadToken() should return nil for non-existent file, got: %v", token)
	}
}

