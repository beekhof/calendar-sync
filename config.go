package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds the configuration for the calendar sync tool.
type Config struct {
	WorkTokenPath       string `json:"work_token_path,omitempty"`
	PersonalTokenPath   string `json:"personal_token_path,omitempty"`
	SyncCalendarName    string `json:"sync_calendar_name,omitempty"`
	SyncCalendarColorID string `json:"sync_calendar_color_id,omitempty"`
	GoogleClientID      string `json:"google_client_id,omitempty"`
	GoogleClientSecret  string `json:"google_client_secret,omitempty"`
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
func LoadConfig(configFile string, workTokenPathFlag, personalTokenPathFlag, syncCalendarNameFlag, syncCalendarColorIDFlag, googleClientIDFlag, googleClientSecretFlag string) (*Config, error) {
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
	// Secrets can be overridden by environment variables (even if in config file)
	if googleClientID := os.Getenv("GOOGLE_CLIENT_ID"); googleClientID != "" {
		config.GoogleClientID = googleClientID
	}
	if googleClientSecret := os.Getenv("GOOGLE_CLIENT_SECRET"); googleClientSecret != "" {
		config.GoogleClientSecret = googleClientSecret
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
	if googleClientIDFlag != "" {
		config.GoogleClientID = googleClientIDFlag
	}
	if googleClientSecretFlag != "" {
		config.GoogleClientSecret = googleClientSecretFlag
	}

	// Step 4: Apply defaults and validate required fields
	if config.WorkTokenPath == "" {
		return nil, fmt.Errorf("work_token_path must be provided via --work-token-path flag, WORK_TOKEN_PATH environment variable, or config file")
	}

	if config.PersonalTokenPath == "" {
		return nil, fmt.Errorf("personal_token_path must be provided via --personal-token-path flag, PERSONAL_TOKEN_PATH environment variable, or config file")
	}

	if config.SyncCalendarName == "" {
		config.SyncCalendarName = "Work Sync"
	}

	if config.SyncCalendarColorID == "" {
		config.SyncCalendarColorID = "7"
	}

	return &config, nil
}

