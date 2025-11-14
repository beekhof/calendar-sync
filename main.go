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
    --config FILE                 Path to JSON config file (optional)
                                  All settings can be specified in the config file
    --work-token-path PATH        Path to store the work account OAuth token
                                  (overrides config file and WORK_TOKEN_PATH env var)
    --personal-token-path PATH    Path to store the personal account OAuth token
                                  (overrides config file and PERSONAL_TOKEN_PATH env var)
    --sync-calendar-name NAME     Name of the calendar to create/use
                                  (default: "Work Sync", overrides config file and SYNC_CALENDAR_NAME env var)
    --sync-calendar-color-id ID   Color ID for the sync calendar
                                  (default: "7" for Grape, overrides config file and SYNC_CALENDAR_COLOR_ID env var)
    --google-credentials-path PATH Path to Google OAuth credentials JSON file
                                  (overrides config file and GOOGLE_CREDENTIALS_PATH env var)
    --destination-type TYPE       Destination calendar type: 'google' or 'apple'
                                  (default: 'google', overrides config file and DESTINATION_TYPE env var)
    --apple-caldav-server-url URL  Apple CalDAV server URL (e.g., 'https://caldav.icloud.com')
                                  (overrides config file and APPLE_CALDAV_SERVER_URL env var)
    --apple-caldav-username USER   Apple CalDAV username (iCloud email)
                                  (overrides config file and APPLE_CALDAV_USERNAME env var)
    --apple-caldav-password PASS   Apple CalDAV password (app-specific password)
                                  (overrides config file and APPLE_CALDAV_PASSWORD env var)

CONFIGURATION PRECEDENCE (highest to lowest):
    1. Command-line flags
    2. Environment variables
    3. Config file (--config)
    4. Defaults

CONFIG FILE:
    All settings can be specified in a JSON config file. Example for Google Calendar:
    {
      "work_token_path": "/path/to/work_token.json",
      "personal_token_path": "/path/to/personal_token.json",
      "sync_calendar_name": "Work Sync",
      "sync_calendar_color_id": "7",
      "google_credentials_path": "/path/to/credentials.json",
      "destination_type": "google"
    }
    
    Example for Apple Calendar destination:
    {
      "work_token_path": "/path/to/work_token.json",
      "sync_calendar_name": "Work Sync",
      "google_credentials_path": "/path/to/credentials.json",
      "destination_type": "apple",
      "apple_caldav_server_url": "https://caldav.icloud.com",
      "apple_caldav_username": "your-email@icloud.com",
      "apple_caldav_password": "app-specific-password"
    }
    
    The Google credentials JSON file should be in the format downloaded from
    Google Cloud Console. It should contain either an "installed" or "web"
    section with "client_id" and "client_secret" fields.
    
    For Apple Calendar, you need an app-specific password from iCloud.
    Generate one at: https://appleid.apple.com/account/manage

ENVIRONMENT VARIABLES:
    All settings can be provided via environment variables:
        WORK_TOKEN_PATH          Path to store the work account OAuth token
        PERSONAL_TOKEN_PATH       Path to store the personal account OAuth token (Google only)
        SYNC_CALENDAR_NAME        Name of the calendar to create/use (default: "Work Sync")
        SYNC_CALENDAR_COLOR_ID    Color ID for the sync calendar (default: "7" for Grape)
        GOOGLE_CREDENTIALS_PATH   Path to Google OAuth credentials JSON file
        DESTINATION_TYPE          Destination calendar type: 'google' or 'apple' (default: 'google')
        APPLE_CALDAV_SERVER_URL   Apple CalDAV server URL (Apple only)
        APPLE_CALDAV_USERNAME      Apple CalDAV username (Apple only)
        APPLE_CALDAV_PASSWORD      Apple CalDAV password (Apple only)

DESCRIPTION:
    This tool performs a one-way sync from your work Google Calendar to your
    personal calendar (Google Calendar or Apple Calendar/iCloud). It creates a
    separate "Work Sync" calendar in your destination account and populates it
    with filtered events from your work calendar.

    The tool syncs events within a two-week rolling window (current week + next week)
    and applies the following filters:
    - All all-day events are synced (including Out of Office)
    - Timed events between 6:00 AM and 12:00 AM (midnight) are synced
    - Timed Out of Office events are skipped
    - Recurring events are expanded to individual instances

    On first run, you will be prompted to authorize both accounts via OAuth 2.0.
    Subsequent runs use stored refresh tokens.

EXAMPLES:
    # Run the sync with a config file
    %s --config /path/to/config.json

    # Run the sync with config file, but override credentials path via environment
    GOOGLE_CREDENTIALS_PATH="/path/to/creds.json" %s --config /path/to/config.json

    # Run the sync with environment variables
    %s

    # Run the sync with command-line flags
    %s --work-token-path /path/to/work_token.json \\
       --personal-token-path /path/to/personal_token.json \\
       --sync-calendar-name "My Work Sync" \\
       --sync-calendar-color-id "7" \\
       --google-credentials-path /path/to/credentials.json

    # Mix config file and command-line flags
    %s --config /path/to/config.json --sync-calendar-name "Custom Name"

    # Show help
    %s --help

`, os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func main() {
	// Parse command-line flags
	helpFlag := flag.Bool("help", false, "Show help message")
	helpFlagShort := flag.Bool("h", false, "Show help message (shorthand)")
	configFile := flag.String("config", "", "Path to JSON config file (optional)")
	workTokenPath := flag.String("work-token-path", "", "Path to store the work account OAuth token")
	personalTokenPath := flag.String("personal-token-path", "", "Path to store the personal account OAuth token")
	syncCalendarName := flag.String("sync-calendar-name", "", "Name of the calendar to create/use (default: \"Work Sync\")")
	syncCalendarColorID := flag.String("sync-calendar-color-id", "", "Color ID for the sync calendar (default: \"7\" for Grape)")
	googleCredentialsPath := flag.String("google-credentials-path", "", "Path to Google OAuth credentials JSON file (overrides config file and GOOGLE_CREDENTIALS_PATH env var)")
	destinationType := flag.String("destination-type", "", "Destination calendar type: 'google' or 'apple' (default: 'google')")
	appleCalDAVServerURL := flag.String("apple-caldav-server-url", "", "Apple CalDAV server URL (e.g., 'https://caldav.icloud.com')")
	appleCalDAVUsername := flag.String("apple-caldav-username", "", "Apple CalDAV username (iCloud email)")
	appleCalDAVPassword := flag.String("apple-caldav-password", "", "Apple CalDAV password (app-specific password)")
	flag.Parse()

	// Show help if requested
	if *helpFlag || *helpFlagShort {
		printHelp()
		os.Exit(0)
	}

	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	ctx := context.Background()

	// Load configuration (precedence: flags > env vars > config file > defaults)
	config, err := LoadConfig(*configFile, *workTokenPath, *personalTokenPath, *syncCalendarName, *syncCalendarColorID, *googleCredentialsPath, *destinationType, *appleCalDAVServerURL, *appleCalDAVUsername, *appleCalDAVPassword)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Work calendar is always Google Calendar (source)
	// Load Google OAuth credentials from the credentials file
	clientID, clientSecret, err := LoadGoogleCredentials(config.GoogleCredentialsPath)
	if err != nil {
		log.Fatalf("Failed to load Google credentials: %v", err)
	}

	googleOAuthConfig := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  "http://127.0.0.1:8080", // Will be updated dynamically by auth flow
		Scopes: []string{
			calendar.CalendarScope,
			calendar.CalendarEventsScope,
		},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
	}

	// Create the work token store (always Google)
	workTokenStore := NewFileTokenStore(config.WorkTokenPath)

	// Get the authenticated work client (always Google)
	workHTTPClient, err := GetAuthenticatedClient(ctx, googleOAuthConfig, workTokenStore)
	if err != nil {
		log.Fatalf("Failed to authenticate work account: %v", err)
	}

	// Create the work calendar client (always Google)
	workClient, err := NewClient(ctx, workHTTPClient)
	if err != nil {
		log.Fatalf("Failed to create work calendar client: %v", err)
	}

	// Create the personal/destination calendar client based on destination type
	var personalClient CalendarClient
	if config.DestinationType == "apple" {
		// Create Apple Calendar client using CalDAV
		personalClient, err = NewAppleCalendarClient(ctx, config.AppleCalDAVServerURL, config.AppleCalDAVUsername, config.AppleCalDAVPassword)
		if err != nil {
			log.Fatalf("Failed to create Apple Calendar client: %v", err)
		}
	} else {
		// Default to Google Calendar
		personalTokenStore := NewFileTokenStore(config.PersonalTokenPath)
		personalHTTPClient, err := GetAuthenticatedClient(ctx, googleOAuthConfig, personalTokenStore)
		if err != nil {
			log.Fatalf("Failed to authenticate personal account: %v", err)
		}

		personalClient, err = NewClient(ctx, personalHTTPClient)
		if err != nil {
			log.Fatalf("Failed to create personal calendar client: %v", err)
		}
	}

	// Create the Syncer
	syncer := NewSyncer(workClient, personalClient, config)

	// Run the sync
	if err := syncer.Sync(ctx); err != nil {
		log.Fatalf("Sync failed: %v", err)
	}

	log.Println("Sync completed successfully.")
}
