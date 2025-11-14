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

	// Test loading from environment variables (empty flags)
	config, err := LoadConfig("", "", "", "")
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

func TestLoadConfig_CommandLineFlags(t *testing.T) {
	// Test that command-line flags override environment variables
	t.Setenv("WORK_TOKEN_PATH", "/env/work_token.json")
	t.Setenv("PERSONAL_TOKEN_PATH", "/env/personal_token.json")

	// Provide flags that should override env vars
	config, err := LoadConfig("/flag/work_token.json", "/flag/personal_token.json", "Flag Calendar", "5")
	if err != nil {
		t.Fatalf("LoadConfig() returned an error: %v", err)
	}

	if config.WorkTokenPath != "/flag/work_token.json" {
		t.Errorf("Expected WorkTokenPath to be '/flag/work_token.json', got '%s'", config.WorkTokenPath)
	}

	if config.PersonalTokenPath != "/flag/personal_token.json" {
		t.Errorf("Expected PersonalTokenPath to be '/flag/personal_token.json', got '%s'", config.PersonalTokenPath)
	}

	if config.SyncCalendarName != "Flag Calendar" {
		t.Errorf("Expected SyncCalendarName to be 'Flag Calendar', got '%s'", config.SyncCalendarName)
	}

	if config.SyncCalendarColorID != "5" {
		t.Errorf("Expected SyncCalendarColorID to be '5', got '%s'", config.SyncCalendarColorID)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	// Set required token paths but not calendar name/color
	t.Setenv("WORK_TOKEN_PATH", "/tmp/work_token.json")
	t.Setenv("PERSONAL_TOKEN_PATH", "/tmp/personal_token.json")
	os.Clearenv()
	t.Setenv("WORK_TOKEN_PATH", "/tmp/work_token.json")
	t.Setenv("PERSONAL_TOKEN_PATH", "/tmp/personal_token.json")

	// Test that defaults are used when neither flag nor env var is set for calendar name/color
	config, err := LoadConfig("", "", "", "")
	if err != nil {
		t.Fatalf("LoadConfig() returned an error: %v", err)
	}

	// Should use defaults for calendar name and color ID
	if config.SyncCalendarName != "Work Sync" {
		t.Errorf("Expected SyncCalendarName to default to 'Work Sync', got '%s'", config.SyncCalendarName)
	}

	if config.SyncCalendarColorID != "7" {
		t.Errorf("Expected SyncCalendarColorID to default to '7', got '%s'", config.SyncCalendarColorID)
	}
}

func TestLoadConfigMissing(t *testing.T) {
	// Clear all environment variables
	os.Clearenv()

	// Try to load config without setting any variables or flags
	config, err := LoadConfig("", "", "", "")
	if err == nil {
		t.Error("LoadConfig() should have returned an error when required token paths are missing")
	}
	if config != nil {
		t.Error("LoadConfig() should have returned nil config when there's an error")
	}
}

