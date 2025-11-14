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

// LoadConfig loads configuration from command-line flags or environment variables.
// Command-line flags take precedence over environment variables.
// Returns an error if any required value is missing.
func LoadConfig(workTokenPathFlag, personalTokenPathFlag, syncCalendarNameFlag, syncCalendarColorIDFlag string) (*Config, error) {
	// Use flag value if provided, otherwise fall back to environment variable
	workTokenPath := workTokenPathFlag
	if workTokenPath == "" {
		workTokenPath = os.Getenv("WORK_TOKEN_PATH")
	}
	if workTokenPath == "" {
		return nil, fmt.Errorf("WORK_TOKEN_PATH must be provided via --work-token-path flag or WORK_TOKEN_PATH environment variable")
	}

	personalTokenPath := personalTokenPathFlag
	if personalTokenPath == "" {
		personalTokenPath = os.Getenv("PERSONAL_TOKEN_PATH")
	}
	if personalTokenPath == "" {
		return nil, fmt.Errorf("PERSONAL_TOKEN_PATH must be provided via --personal-token-path flag or PERSONAL_TOKEN_PATH environment variable")
	}

	syncCalendarName := syncCalendarNameFlag
	if syncCalendarName == "" {
		syncCalendarName = os.Getenv("SYNC_CALENDAR_NAME")
	}
	if syncCalendarName == "" {
		// Default to "Work Sync" if not specified
		syncCalendarName = "Work Sync"
	}

	syncCalendarColorID := syncCalendarColorIDFlag
	if syncCalendarColorID == "" {
		syncCalendarColorID = os.Getenv("SYNC_CALENDAR_COLOR_ID")
	}
	if syncCalendarColorID == "" {
		// Default to "7" (Grape) if not specified
		syncCalendarColorID = "7"
	}

	return &Config{
		WorkTokenPath:       workTokenPath,
		PersonalTokenPath:   personalTokenPath,
		SyncCalendarName:    syncCalendarName,
		SyncCalendarColorID: syncCalendarColorID,
	}, nil
}

