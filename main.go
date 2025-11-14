package main

import (
	"context"
	"log"
	"os"

	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
)

func main() {
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	ctx := context.Background()

	// Load configuration
	config, err := LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Define OAuth 2.0 configuration
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	if clientID == "" {
		log.Fatal("GOOGLE_CLIENT_ID environment variable is required")
	}

	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if clientSecret == "" {
		log.Fatal("GOOGLE_CLIENT_SECRET environment variable is required")
	}

	googleOAuthConfig := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob",
		Scopes: []string{
			calendar.CalendarScope,
			calendar.CalendarEventsScope,
		},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
	}

	// Create the two token stores
	workTokenStore := NewFileTokenStore(config.WorkTokenPath)
	personalTokenStore := NewFileTokenStore(config.PersonalTokenPath)

	// Get the two authenticated clients
	workHTTPClient, err := GetAuthenticatedClient(ctx, googleOAuthConfig, workTokenStore)
	if err != nil {
		log.Fatalf("Failed to authenticate work account: %v", err)
	}

	personalHTTPClient, err := GetAuthenticatedClient(ctx, googleOAuthConfig, personalTokenStore)
	if err != nil {
		log.Fatalf("Failed to authenticate personal account: %v", err)
	}

	// Create the two high-level Google Calendar clients
	workClient, err := NewClient(ctx, workHTTPClient)
	if err != nil {
		log.Fatalf("Failed to create work calendar client: %v", err)
	}

	personalClient, err := NewClient(ctx, personalHTTPClient)
	if err != nil {
		log.Fatalf("Failed to create personal calendar client: %v", err)
	}

	// Create the Syncer
	syncer := NewSyncer(workClient, personalClient, config)

	// Run the sync
	if err := syncer.Sync(ctx); err != nil {
		log.Fatalf("Sync failed: %v", err)
	}

	log.Println("Sync completed successfully.")
}
