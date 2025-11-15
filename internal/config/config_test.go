package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Set all required environment variables
	t.Setenv("WORK_TOKEN_PATH", "/tmp/work_token.json")
	t.Setenv("PERSONAL_TOKEN_PATH", "/tmp/personal_token.json")
	t.Setenv("GOOGLE_CREDENTIALS_PATH", "/tmp/credentials.json")
	t.Setenv("SYNC_CALENDAR_NAME", "Work Sync")
	t.Setenv("SYNC_CALENDAR_COLOR_ID", "7")

	// Test loading from environment variables (empty flags and no config file)
	config, err := LoadConfig("", "", "", "", "", "", "", "", "", "")
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
	t.Setenv("GOOGLE_CREDENTIALS_PATH", "/env/credentials.json")

	// Provide flags that should override env vars
	config, err := LoadConfig("", "/flag/work_token.json", "/flag/personal_token.json", "Flag Calendar", "5", "/flag/credentials.json", "", "", "", "")
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
	t.Setenv("GOOGLE_CREDENTIALS_PATH", "/tmp/credentials.json")
	os.Clearenv()
	t.Setenv("WORK_TOKEN_PATH", "/tmp/work_token.json")
	t.Setenv("PERSONAL_TOKEN_PATH", "/tmp/personal_token.json")
	t.Setenv("GOOGLE_CREDENTIALS_PATH", "/tmp/credentials.json")

	// Test that defaults are used when neither flag nor env var is set for calendar name/color
	config, err := LoadConfig("", "", "", "", "", "", "", "", "", "")
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

func TestLoadConfig_ConfigFile(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	
	configJSON := `{
		"work_token_path": "/config/work_token.json",
		"personal_token_path": "/config/personal_token.json",
		"sync_calendar_name": "Config Calendar",
		"sync_calendar_color_id": "3",
		"google_credentials_path": "/config/credentials.json"
	}`
	
	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config from file
	config, err := LoadConfig(configPath, "", "", "", "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("LoadConfig() returned an error: %v", err)
	}

	if config.WorkTokenPath != "/config/work_token.json" {
		t.Errorf("Expected WorkTokenPath to be '/config/work_token.json', got '%s'", config.WorkTokenPath)
	}

	if config.PersonalTokenPath != "/config/personal_token.json" {
		t.Errorf("Expected PersonalTokenPath to be '/config/personal_token.json', got '%s'", config.PersonalTokenPath)
	}

	if config.SyncCalendarName != "Config Calendar" {
		t.Errorf("Expected SyncCalendarName to be 'Config Calendar', got '%s'", config.SyncCalendarName)
	}

	if config.SyncCalendarColorID != "3" {
		t.Errorf("Expected SyncCalendarColorID to be '3', got '%s'", config.SyncCalendarColorID)
	}

	if config.GoogleCredentialsPath != "/config/credentials.json" {
		t.Errorf("Expected GoogleCredentialsPath to be '/config/credentials.json', got '%s'", config.GoogleCredentialsPath)
	}
}

func TestLoadConfig_EnvVarsOverrideConfigFile(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	
	configJSON := `{
		"work_token_path": "/config/work_token.json",
		"personal_token_path": "/config/personal_token.json",
		"google_credentials_path": "/config/credentials.json"
	}`
	
	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set environment variable that should override config file
	t.Setenv("GOOGLE_CREDENTIALS_PATH", "/env/credentials.json")

	// Load config - env var should override config file
	config, err := LoadConfig(configPath, "", "", "", "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("LoadConfig() returned an error: %v", err)
	}

	// These should come from config file
	if config.WorkTokenPath != "/config/work_token.json" {
		t.Errorf("Expected WorkTokenPath from config file, got '%s'", config.WorkTokenPath)
	}

	// This should be overridden by environment variable
	if config.GoogleCredentialsPath != "/env/credentials.json" {
		t.Errorf("Expected GoogleCredentialsPath to be overridden by env var '/env/credentials.json', got '%s'", config.GoogleCredentialsPath)
	}
}

func TestLoadConfigMissing(t *testing.T) {
	// Clear all environment variables
	os.Clearenv()

	// Try to load config without setting any variables or flags
	config, err := LoadConfig("", "", "", "", "", "", "", "", "", "")
	if err == nil {
		t.Error("LoadConfig() should have returned an error when required token paths are missing")
	}
	if config != nil {
		t.Error("LoadConfig() should have returned nil config when there's an error")
	}
}

func TestLoadGoogleCredentials_Installed(t *testing.T) {
	// Create a temporary credentials file with "installed" format
	tempDir := t.TempDir()
	credsPath := filepath.Join(tempDir, "credentials.json")
	
	credsJSON := `{
		"installed": {
			"client_id": "test-client-id",
			"client_secret": "test-client-secret"
		}
	}`
	
	if err := os.WriteFile(credsPath, []byte(credsJSON), 0644); err != nil {
		t.Fatalf("Failed to write credentials file: %v", err)
	}

	clientID, clientSecret, err := LoadGoogleCredentials(credsPath)
	if err != nil {
		t.Fatalf("LoadGoogleCredentials() returned an error: %v", err)
	}

	if clientID != "test-client-id" {
		t.Errorf("Expected clientID to be 'test-client-id', got '%s'", clientID)
	}

	if clientSecret != "test-client-secret" {
		t.Errorf("Expected clientSecret to be 'test-client-secret', got '%s'", clientSecret)
	}
}

func TestLoadGoogleCredentials_Web(t *testing.T) {
	// Create a temporary credentials file with "web" format
	tempDir := t.TempDir()
	credsPath := filepath.Join(tempDir, "credentials.json")
	
	credsJSON := `{
		"web": {
			"client_id": "web-client-id",
			"client_secret": "web-client-secret"
		}
	}`
	
	if err := os.WriteFile(credsPath, []byte(credsJSON), 0644); err != nil {
		t.Fatalf("Failed to write credentials file: %v", err)
	}

	clientID, clientSecret, err := LoadGoogleCredentials(credsPath)
	if err != nil {
		t.Fatalf("LoadGoogleCredentials() returned an error: %v", err)
	}

	if clientID != "web-client-id" {
		t.Errorf("Expected clientID to be 'web-client-id', got '%s'", clientID)
	}

	if clientSecret != "web-client-secret" {
		t.Errorf("Expected clientSecret to be 'web-client-secret', got '%s'", clientSecret)
	}
}

