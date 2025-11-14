package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
)

func printHelp() {
	fmt.Fprintf(os.Stderr, `Calendar Sync Tool

A one-way synchronization tool that syncs events from a work Google Calendar
to a personal Google Calendar, creating a read-only "Work Sync" calendar.

USAGE:
    %s [OPTIONS]

OPTIONS:
    -h, --help                    Show this help message and exit
    --work-token-path PATH        Path to store the work account OAuth token
                                  (overrides WORK_TOKEN_PATH env var)
    --personal-token-path PATH    Path to store the personal account OAuth token
                                  (overrides PERSONAL_TOKEN_PATH env var)
    --sync-calendar-name NAME     Name of the calendar to create/use
                                  (default: "Work Sync", overrides SYNC_CALENDAR_NAME env var)
    --sync-calendar-color-id ID   Color ID for the sync calendar
                                  (default: "7" for Grape, overrides SYNC_CALENDAR_COLOR_ID env var)

ENVIRONMENT VARIABLES:
    Required (if not provided via command-line flags):
        WORK_TOKEN_PATH          Path to store the work account OAuth token
        PERSONAL_TOKEN_PATH       Path to store the personal account OAuth token
        SYNC_CALENDAR_NAME        Name of the calendar to create/use (default: "Work Sync")
        SYNC_CALENDAR_COLOR_ID    Color ID for the sync calendar (default: "7" for Grape)
    
    Always Required:
        GOOGLE_CLIENT_ID          Google OAuth 2.0 Client ID
        GOOGLE_CLIENT_SECRET      Google OAuth 2.0 Client Secret

DESCRIPTION:
    This tool performs a one-way sync from your work Google Calendar to your
    personal Google Calendar. It creates a separate "Work Sync" calendar in your
    personal account and populates it with filtered events from your work calendar.

    The tool syncs events within a two-week rolling window (current week + next week)
    and applies the following filters:
    - All all-day events are synced (including Out of Office)
    - Timed events between 6:00 AM and 12:00 AM (midnight) are synced
    - Timed Out of Office events are skipped
    - Recurring events are expanded to individual instances

    On first run, you will be prompted to authorize both accounts via OAuth 2.0.
    Subsequent runs use stored refresh tokens.

EXAMPLES:
    # Run the sync with environment variables
    %s

    # Run the sync with command-line flags
    %s --work-token-path /path/to/work_token.json \\
       --personal-token-path /path/to/personal_token.json \\
       --sync-calendar-name "My Work Sync" \\
       --sync-calendar-color-id "7"

    # Mix command-line flags and environment variables
    %s --work-token-path /path/to/work_token.json

    # Show help
    %s --help

`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func main() {
	// Parse command-line flags
	helpFlag := flag.Bool("help", false, "Show help message")
	helpFlagShort := flag.Bool("h", false, "Show help message (shorthand)")
	workTokenPath := flag.String("work-token-path", "", "Path to store the work account OAuth token")
	personalTokenPath := flag.String("personal-token-path", "", "Path to store the personal account OAuth token")
	syncCalendarName := flag.String("sync-calendar-name", "", "Name of the calendar to create/use (default: \"Work Sync\")")
	syncCalendarColorID := flag.String("sync-calendar-color-id", "", "Color ID for the sync calendar (default: \"7\" for Grape)")
	flag.Parse()

	// Show help if requested
	if *helpFlag || *helpFlagShort {
		printHelp()
		os.Exit(0)
	}

	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	ctx := context.Background()

	// Load configuration (command-line flags take precedence over environment variables)
	config, err := LoadConfig(*workTokenPath, *personalTokenPath, *syncCalendarName, *syncCalendarColorID)
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
