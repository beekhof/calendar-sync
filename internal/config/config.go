package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// GoogleCredentials represents the structure of Google OAuth credentials JSON file.
type GoogleCredentials struct {
	Installed struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"installed"`
	Web struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"web"`
}

// LoadGoogleCredentials loads Google OAuth credentials from a JSON file.
func LoadGoogleCredentials(path string) (clientID, clientSecret string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("failed to read credentials file: %w", err)
	}

	var creds GoogleCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", "", fmt.Errorf("failed to parse credentials file: %w", err)
	}

	// Try "installed" first (for desktop apps), then "web"
	if creds.Installed.ClientID != "" {
		return creds.Installed.ClientID, creds.Installed.ClientSecret, nil
	}
	if creds.Web.ClientID != "" {
		return creds.Web.ClientID, creds.Web.ClientSecret, nil
	}

	return "", "", fmt.Errorf("no client_id found in credentials file (expected 'installed' or 'web' section)")
}

// Destination represents a single destination calendar configuration.
type Destination struct {
	Name            string `json:"name"`                        // Name for logging (e.g., "Personal Google", "iCloud")
	Type            string `json:"type"`                        // "google" or "apple"
	TokenPath       string `json:"token_path,omitempty"`         // For Google: path to OAuth token file
	CalendarName    string `json:"calendar_name,omitempty"`     // Name of the calendar to create/use
	CalendarColorID string `json:"calendar_color_id,omitempty"` // Color ID for the calendar

	// Apple Calendar specific fields
	ServerURL string `json:"server_url,omitempty"` // CalDAV server URL (e.g., "https://caldav.icloud.com")
	Username  string `json:"username,omitempty"`   // iCloud email
	Password  string `json:"password,omitempty"`   // App-specific password
}

// Config holds the configuration for the calendar sync tool.
type Config struct {
	WorkTokenPath         string `json:"work_token_path,omitempty"`
	PersonalTokenPath     string `json:"personal_token_path,omitempty"` // Deprecated: use destinations[].token_path
	SyncCalendarName      string `json:"sync_calendar_name,omitempty"`   // Deprecated: use destinations[].calendar_name
	SyncCalendarColorID   string `json:"sync_calendar_color_id,omitempty"` // Deprecated: use destinations[].calendar_color_id
	GoogleCredentialsPath string `json:"google_credentials_path,omitempty"`

	// Apple Calendar destination configuration (deprecated: use destinations[])
	DestinationType      string `json:"destination_type,omitempty"`        // "google" or "apple"
	AppleCalDAVServerURL string `json:"apple_caldav_server_url,omitempty"` // e.g., "https://caldav.icloud.com"
	AppleCalDAVUsername  string `json:"apple_caldav_username,omitempty"`   // iCloud email
	AppleCalDAVPassword  string `json:"apple_caldav_password,omitempty"`   // App-specific password

	// Multiple destinations support
	Destinations []Destination `json:"destinations,omitempty"` // Array of destination configurations

	// Sync window configuration
	SyncWindowWeeks     int `json:"sync_window_weeks,omitempty"`      // Number of weeks to sync forward from start of current week (default: 2)
	SyncWindowWeeksPast int `json:"sync_window_weeks_past,omitempty"` // Number of weeks to sync backward from start of current week (default: 0)
}

// LoadConfigFromFile loads configuration from a JSON file.
func LoadConfigFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// LoadConfig loads configuration with the following precedence (highest to lowest):
// 1. Command-line flags
// 2. Environment variables
// 3. Config file
// 4. Defaults
// Returns an error if any required value is missing.
func LoadConfig(configFile string, workTokenPathFlag, personalTokenPathFlag, syncCalendarNameFlag, syncCalendarColorIDFlag, googleCredentialsPathFlag, destinationTypeFlag, appleCalDAVServerURLFlag, appleCalDAVUsernameFlag, appleCalDAVPasswordFlag string) (*Config, error) {
	var config Config

	// Step 1: Load from config file if provided
	if configFile != "" {
		fileConfig, err := LoadConfigFromFile(configFile)
		if err != nil {
			return nil, err
		}
		config = *fileConfig
	}

	// Step 2: Override with environment variables
	if workTokenPath := os.Getenv("WORK_TOKEN_PATH"); workTokenPath != "" {
		config.WorkTokenPath = workTokenPath
	}
	if personalTokenPath := os.Getenv("PERSONAL_TOKEN_PATH"); personalTokenPath != "" {
		config.PersonalTokenPath = personalTokenPath
	}
	if syncCalendarName := os.Getenv("SYNC_CALENDAR_NAME"); syncCalendarName != "" {
		config.SyncCalendarName = syncCalendarName
	}
	if syncCalendarColorID := os.Getenv("SYNC_CALENDAR_COLOR_ID"); syncCalendarColorID != "" {
		config.SyncCalendarColorID = syncCalendarColorID
	}
	// Credentials path can be overridden by environment variable
	if googleCredentialsPath := os.Getenv("GOOGLE_CREDENTIALS_PATH"); googleCredentialsPath != "" {
		config.GoogleCredentialsPath = googleCredentialsPath
	}
	if destinationType := os.Getenv("DESTINATION_TYPE"); destinationType != "" {
		config.DestinationType = destinationType
	}
	if appleCalDAVServerURL := os.Getenv("APPLE_CALDAV_SERVER_URL"); appleCalDAVServerURL != "" {
		config.AppleCalDAVServerURL = appleCalDAVServerURL
	}
	if appleCalDAVUsername := os.Getenv("APPLE_CALDAV_USERNAME"); appleCalDAVUsername != "" {
		config.AppleCalDAVUsername = appleCalDAVUsername
	}
	if appleCalDAVPassword := os.Getenv("APPLE_CALDAV_PASSWORD"); appleCalDAVPassword != "" {
		config.AppleCalDAVPassword = appleCalDAVPassword
	}
	// Sync window weeks from environment variable
	if syncWindowWeeks := os.Getenv("SYNC_WINDOW_WEEKS"); syncWindowWeeks != "" {
		var err error
		if config.SyncWindowWeeks, err = parseInt(syncWindowWeeks); err != nil {
			return nil, fmt.Errorf("invalid SYNC_WINDOW_WEEKS value: %w", err)
		}
	}
	// Sync window weeks past from environment variable
	if syncWindowWeeksPast := os.Getenv("SYNC_WINDOW_WEEKS_PAST"); syncWindowWeeksPast != "" {
		var err error
		if config.SyncWindowWeeksPast, err = parseInt(syncWindowWeeksPast); err != nil {
			return nil, fmt.Errorf("invalid SYNC_WINDOW_WEEKS_PAST value: %w", err)
		}
	}

	// Step 3: Override with command-line flags (highest priority)
	if workTokenPathFlag != "" {
		config.WorkTokenPath = workTokenPathFlag
	}
	if personalTokenPathFlag != "" {
		config.PersonalTokenPath = personalTokenPathFlag
	}
	if syncCalendarNameFlag != "" {
		config.SyncCalendarName = syncCalendarNameFlag
	}
	if syncCalendarColorIDFlag != "" {
		config.SyncCalendarColorID = syncCalendarColorIDFlag
	}
	if googleCredentialsPathFlag != "" {
		config.GoogleCredentialsPath = googleCredentialsPathFlag
	}
	if destinationTypeFlag != "" {
		config.DestinationType = destinationTypeFlag
	}
	if appleCalDAVServerURLFlag != "" {
		config.AppleCalDAVServerURL = appleCalDAVServerURLFlag
	}
	if appleCalDAVUsernameFlag != "" {
		config.AppleCalDAVUsername = appleCalDAVUsernameFlag
	}
	if appleCalDAVPasswordFlag != "" {
		config.AppleCalDAVPassword = appleCalDAVPasswordFlag
	}

	// Step 4: Apply defaults and validate required fields
	if config.WorkTokenPath == "" {
		return nil, fmt.Errorf("work_token_path must be provided via --work-token-path flag, WORK_TOKEN_PATH environment variable, or config file")
	}

	if config.GoogleCredentialsPath == "" {
		return nil, fmt.Errorf("google_credentials_path must be provided via --google-credentials-path flag, GOOGLE_CREDENTIALS_PATH environment variable, or config file")
	}

	// Normalize destinations: convert old single-destination format to new array format
	if len(config.Destinations) == 0 {
		// Backward compatibility: create a destination from old format
		destType := config.DestinationType
		if destType == "" {
			destType = "google" // Default to Google
		}

		dest := Destination{
			Name:            "Default",
			Type:            destType,
			CalendarName:    config.SyncCalendarName,
			CalendarColorID: config.SyncCalendarColorID,
		}

		if destType == "google" {
			if config.PersonalTokenPath == "" {
				return nil, fmt.Errorf("personal_token_path must be provided via --personal-token-path flag, PERSONAL_TOKEN_PATH environment variable, or config file (required for Google Calendar destination)")
			}
			dest.TokenPath = config.PersonalTokenPath
		} else if destType == "apple" {
			if config.AppleCalDAVServerURL == "" {
				return nil, fmt.Errorf("apple_caldav_server_url must be provided for Apple Calendar destination")
			}
			if config.AppleCalDAVUsername == "" {
				return nil, fmt.Errorf("apple_caldav_username must be provided for Apple Calendar destination")
			}
			if config.AppleCalDAVPassword == "" {
				return nil, fmt.Errorf("apple_caldav_password must be provided for Apple Calendar destination")
			}
			dest.ServerURL = config.AppleCalDAVServerURL
			dest.Username = config.AppleCalDAVUsername
			dest.Password = config.AppleCalDAVPassword
		} else {
			return nil, fmt.Errorf("destination_type must be 'google' or 'apple', got '%s'", destType)
		}

		config.Destinations = []Destination{dest}
	}

	// Validate and set defaults for each destination
	for i := range config.Destinations {
		dest := &config.Destinations[i]

		// Set default name if not provided
		if dest.Name == "" {
			dest.Name = fmt.Sprintf("Destination %d", i+1)
		}

		// Validate destination type
		if dest.Type != "google" && dest.Type != "apple" {
			return nil, fmt.Errorf("destination[%d].type must be 'google' or 'apple', got '%s'", i, dest.Type)
		}

		// Validate and set defaults based on type
		if dest.Type == "google" {
			if dest.TokenPath == "" {
				return nil, fmt.Errorf("destination[%d] (name: %s): token_path must be provided for Google Calendar destination", i, dest.Name)
			}
		} else if dest.Type == "apple" {
			if dest.ServerURL == "" {
				return nil, fmt.Errorf("destination[%d] (name: %s): server_url must be provided for Apple Calendar destination", i, dest.Name)
			}
			if dest.Username == "" {
				return nil, fmt.Errorf("destination[%d] (name: %s): username must be provided for Apple Calendar destination", i, dest.Name)
			}
			if dest.Password == "" {
				return nil, fmt.Errorf("destination[%d] (name: %s): password must be provided for Apple Calendar destination", i, dest.Name)
			}
		}

		// Set default calendar name and color
		if dest.CalendarName == "" {
			dest.CalendarName = "Work Sync"
		}
		if dest.CalendarColorID == "" {
			dest.CalendarColorID = "7"
		}
	}

	// Default sync window to 2 weeks forward (current week + next week)
	if config.SyncWindowWeeks == 0 {
		config.SyncWindowWeeks = 2
	}

	// Default sync window past to 0 weeks (no past events)
	// No need to set default as 0 is already the zero value

	return &config, nil
}

// parseInt parses a string to an integer.
func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}
