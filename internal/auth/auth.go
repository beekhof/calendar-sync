package auth

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

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

// startLocalServer starts a local HTTP server to receive the OAuth callback.
// Returns the redirect URL, a channel for the authorization code, and a channel for errors.
// Uses port 8080 by default, or a random port if 8080 is unavailable.
func startLocalServer() (string, <-chan string, <-chan error, error) {
	// Try port 8080 first, fall back to random port if unavailable
	listener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		// Fall back to random port if 8080 is in use
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", nil, nil, fmt.Errorf("failed to start local server: %w", err)
		}
	}

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	codeChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	server := &http.Server{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  10 * time.Second,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code != "" {
			fmt.Fprintf(w, "<html><body><h1>Authorization successful!</h1><p>You can close this window.</p></body></html>")
			codeChan <- code
		} else {
			errMsg := r.URL.Query().Get("error")
			if errMsg != "" {
				errorChan <- fmt.Errorf("authorization error: %s", errMsg)
				fmt.Fprintf(w, "<html><body><h1>Authorization failed</h1><p>Error: %s</p></body></html>", errMsg)
			} else {
				fmt.Fprintf(w, "<html><body><h1>No authorization code received</h1></body></html>")
				errorChan <- fmt.Errorf("no authorization code received")
			}
		}
		go func() {
			time.Sleep(1 * time.Second)
			server.Shutdown(context.Background())
		}()
	})
	server.Handler = mux

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errorChan <- fmt.Errorf("server error: %w", err)
		}
	}()

	return redirectURL, codeChan, errorChan, nil
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
		// Start local server to receive callback
		redirectURL, codeChan, errorChan, err := startLocalServer()
		if err != nil {
			return nil, fmt.Errorf("failed to start local server: %w", err)
		}

		// Update the redirect URL in the config
		oauthConfig.RedirectURL = redirectURL

		// Generate auth URL
		authURL := oauthConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
		
		fmt.Printf("Starting local server on %s\n", redirectURL)
		if redirectURL != "http://127.0.0.1:8080" {
			fmt.Printf("Note: Port 8080 was unavailable. Make sure to add %s to your authorized redirect URIs in Google Cloud Console.\n", redirectURL)
		}
		fmt.Println("\nPlease visit the following URL to authorize the application:")
		fmt.Println(authURL)
		fmt.Println("\nWaiting for authorization...")

		// Wait for the authorization code
		var code string
		select {
		case code = <-codeChan:
			// Successfully received code
		case err := <-errorChan:
			return nil, fmt.Errorf("failed to receive authorization code: %w", err)
		case <-time.After(5 * time.Minute):
			return nil, fmt.Errorf("authorization timeout: no response received within 5 minutes")
		}

		if code == "" {
			return nil, fmt.Errorf("no authorization code received")
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

		fmt.Println("Authorization successful!")
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

