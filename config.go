package main

import (
	"fmt"
	"os"
)

// Config holds the configuration for the calendar sync tool.
type Config struct {
	WorkTokenPath       string
	PersonalTokenPath   string
	SyncCalendarName    string
	SyncCalendarColorID string
}

// LoadConfig loads configuration from environment variables.
// Returns an error if any required environment variable is missing.
func LoadConfig() (*Config, error) {
	workTokenPath := os.Getenv("WORK_TOKEN_PATH")
	if workTokenPath == "" {
		return nil, fmt.Errorf("WORK_TOKEN_PATH environment variable is required")
	}

	personalTokenPath := os.Getenv("PERSONAL_TOKEN_PATH")
	if personalTokenPath == "" {
		return nil, fmt.Errorf("PERSONAL_TOKEN_PATH environment variable is required")
	}

	syncCalendarName := os.Getenv("SYNC_CALENDAR_NAME")
	if syncCalendarName == "" {
		return nil, fmt.Errorf("SYNC_CALENDAR_NAME environment variable is required")
	}

	syncCalendarColorID := os.Getenv("SYNC_CALENDAR_COLOR_ID")
	if syncCalendarColorID == "" {
		return nil, fmt.Errorf("SYNC_CALENDAR_COLOR_ID environment variable is required")
	}

	return &Config{
		WorkTokenPath:       workTokenPath,
		PersonalTokenPath:   personalTokenPath,
		SyncCalendarName:    syncCalendarName,
		SyncCalendarColorID: syncCalendarColorID,
	}, nil
}

