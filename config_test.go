package main

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Set all required environment variables
	t.Setenv("WORK_TOKEN_PATH", "/tmp/work_token.json")
	t.Setenv("PERSONAL_TOKEN_PATH", "/tmp/personal_token.json")
	t.Setenv("SYNC_CALENDAR_NAME", "Work Sync")
	t.Setenv("SYNC_CALENDAR_COLOR_ID", "7")

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() returned an error: %v", err)
	}

	if config.WorkTokenPath != "/tmp/work_token.json" {
		t.Errorf("Expected WorkTokenPath to be '/tmp/work_token.json', got '%s'", config.WorkTokenPath)
	}

	if config.PersonalTokenPath != "/tmp/personal_token.json" {
		t.Errorf("Expected PersonalTokenPath to be '/tmp/personal_token.json', got '%s'", config.PersonalTokenPath)
	}

	if config.SyncCalendarName != "Work Sync" {
		t.Errorf("Expected SyncCalendarName to be 'Work Sync', got '%s'", config.SyncCalendarName)
	}

	if config.SyncCalendarColorID != "7" {
		t.Errorf("Expected SyncCalendarColorID to be '7', got '%s'", config.SyncCalendarColorID)
	}
}

func TestLoadConfigMissing(t *testing.T) {
	// Clear all environment variables
	os.Clearenv()

	// Try to load config without setting any variables
	config, err := LoadConfig()
	if err == nil {
		t.Error("LoadConfig() should have returned an error when environment variables are missing")
	}
	if config != nil {
		t.Error("LoadConfig() should have returned nil config when there's an error")
	}
}

