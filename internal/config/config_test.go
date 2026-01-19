package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file with destinations
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	configJSON := `{
		"work_token_path": "/tmp/work_token.json",
		"google_credentials_path": "/tmp/credentials.json",
		"destinations": [
			{
				"name": "Test",
				"type": "google",
				"token_path": "/tmp/personal_token.json",
				"calendar_name": "Work Sync",
				"calendar_color_id": "7"
			}
		]
	}`

	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Test loading from config file
	config, err := LoadConfig(configPath, "", "", "")
	if err != nil {
		t.Fatalf("LoadConfig() returned an error: %v", err)
	}

	if config.WorkTokenPath != "/tmp/work_token.json" {
		t.Errorf("Expected WorkTokenPath to be '/tmp/work_token.json', got '%s'", config.WorkTokenPath)
	}

	if len(config.Destinations) != 1 {
		t.Fatalf("Expected 1 destination, got %d", len(config.Destinations))
	}

	dest := config.Destinations[0]
	if dest.TokenPath != "/tmp/personal_token.json" {
		t.Errorf("Expected destination TokenPath to be '/tmp/personal_token.json', got '%s'", dest.TokenPath)
	}

	if dest.CalendarName != "Work Sync" {
		t.Errorf("Expected destination CalendarName to be 'Work Sync', got '%s'", dest.CalendarName)
	}

	if dest.CalendarColorID != "7" {
		t.Errorf("Expected destination CalendarColorID to be '7', got '%s'", dest.CalendarColorID)
	}
}

func TestLoadConfig_CommandLineFlags(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	configJSON := `{
		"work_token_path": "/config/work_token.json",
		"google_credentials_path": "/config/credentials.json",
		"destinations": [
			{
				"name": "Test",
				"type": "google",
				"token_path": "/config/personal_token.json"
			}
		]
	}`

	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Test that command-line flags override config file
	config, err := LoadConfig(configPath, "/flag/work_token.json", "", "/flag/credentials.json")
	if err != nil {
		t.Fatalf("LoadConfig() returned an error: %v", err)
	}

	if config.WorkTokenPath != "/flag/work_token.json" {
		t.Errorf("Expected WorkTokenPath to be '/flag/work_token.json', got '%s'", config.WorkTokenPath)
	}

	if config.GoogleCredentialsPath != "/flag/credentials.json" {
		t.Errorf("Expected GoogleCredentialsPath to be '/flag/credentials.json', got '%s'", config.GoogleCredentialsPath)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	// Create a temporary config file without calendar name/color
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	configJSON := `{
		"work_token_path": "/tmp/work_token.json",
		"google_credentials_path": "/tmp/credentials.json",
		"destinations": [
			{
				"name": "Test",
				"type": "google",
				"token_path": "/tmp/personal_token.json"
			}
		]
	}`

	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Test that defaults are used when calendar name/color are not specified
	config, err := LoadConfig(configPath, "", "", "")
	if err != nil {
		t.Fatalf("LoadConfig() returned an error: %v", err)
	}

	// Should use defaults for calendar name and color ID in the destination
	if len(config.Destinations) != 1 {
		t.Fatalf("Expected 1 destination, got %d", len(config.Destinations))
	}
	dest := config.Destinations[0]
	if dest.CalendarName != "Work Sync" {
		t.Errorf("Expected destination CalendarName to default to 'Work Sync', got '%s'", dest.CalendarName)
	}

	if dest.CalendarColorID != "7" {
		t.Errorf("Expected destination CalendarColorID to default to '7', got '%s'", dest.CalendarColorID)
	}
}

func TestLoadConfig_ConfigFile(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	configJSON := `{
		"work_token_path": "/config/work_token.json",
		"google_credentials_path": "/config/credentials.json",
		"destinations": [
			{
				"name": "Test",
				"type": "google",
				"token_path": "/config/personal_token.json",
				"calendar_name": "Config Calendar",
				"calendar_color_id": "3"
			}
		]
	}`

	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Load config from file
	config, err := LoadConfig(configPath, "", "", "")
	if err != nil {
		t.Fatalf("LoadConfig() returned an error: %v", err)
	}

	if config.WorkTokenPath != "/config/work_token.json" {
		t.Errorf("Expected WorkTokenPath to be '/config/work_token.json', got '%s'", config.WorkTokenPath)
	}

	if len(config.Destinations) != 1 {
		t.Fatalf("Expected 1 destination, got %d", len(config.Destinations))
	}

	dest := config.Destinations[0]
	if dest.TokenPath != "/config/personal_token.json" {
		t.Errorf("Expected destination TokenPath to be '/config/personal_token.json', got '%s'", dest.TokenPath)
	}

	if dest.CalendarName != "Config Calendar" {
		t.Errorf("Expected destination CalendarName to be 'Config Calendar', got '%s'", dest.CalendarName)
	}

	if dest.CalendarColorID != "3" {
		t.Errorf("Expected destination CalendarColorID to be '3', got '%s'", dest.CalendarColorID)
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
		"google_credentials_path": "/config/credentials.json",
		"destinations": [
			{
				"name": "Test",
				"type": "google",
				"token_path": "/config/personal_token.json"
			}
		]
	}`

	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set environment variable that should override config file
	t.Setenv("GOOGLE_CREDENTIALS_PATH", "/env/credentials.json")

	// Load config - env var should override config file
	config, err := LoadConfig(configPath, "", "", "")
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

	// Try to load config without a config file (config file is required)
	config, err := LoadConfig("", "", "", "")
	if err == nil {
		t.Error("LoadConfig() should have returned an error when config file is missing")
	}
	if config != nil {
		t.Error("LoadConfig() should have returned nil config when there's an error")
	}
}

func TestLoadConfigMissingDestinations(t *testing.T) {
	// Create a temporary config file without destinations
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	configJSON := `{
		"work_token_path": "/tmp/work_token.json",
		"google_credentials_path": "/tmp/credentials.json"
	}`

	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Try to load config without destinations array
	config, err := LoadConfig(configPath, "", "", "")
	if err == nil {
		t.Error("LoadConfig() should have returned an error when destinations array is missing")
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
