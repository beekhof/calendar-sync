package main

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

// Config holds the configuration for the calendar sync tool.
type Config struct {
	WorkTokenPath          string `json:"work_token_path,omitempty"`
	PersonalTokenPath      string `json:"personal_token_path,omitempty"`
	SyncCalendarName       string `json:"sync_calendar_name,omitempty"`
	SyncCalendarColorID    string `json:"sync_calendar_color_id,omitempty"`
	GoogleCredentialsPath  string `json:"google_credentials_path,omitempty"`
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
func LoadConfig(configFile string, workTokenPathFlag, personalTokenPathFlag, syncCalendarNameFlag, syncCalendarColorIDFlag, googleCredentialsPathFlag string) (*Config, error) {
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

	// Step 4: Apply defaults and validate required fields
	if config.WorkTokenPath == "" {
		return nil, fmt.Errorf("work_token_path must be provided via --work-token-path flag, WORK_TOKEN_PATH environment variable, or config file")
	}

	if config.PersonalTokenPath == "" {
		return nil, fmt.Errorf("personal_token_path must be provided via --personal-token-path flag, PERSONAL_TOKEN_PATH environment variable, or config file")
	}

	if config.GoogleCredentialsPath == "" {
		return nil, fmt.Errorf("google_credentials_path must be provided via --google-credentials-path flag, GOOGLE_CREDENTIALS_PATH environment variable, or config file")
	}

	if config.SyncCalendarName == "" {
		config.SyncCalendarName = "Work Sync"
	}

	if config.SyncCalendarColorID == "" {
		config.SyncCalendarColorID = "7"
	}

	return &config, nil
}

