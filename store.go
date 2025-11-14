package main

import (
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/oauth2"
)

// FileTokenStore is a file-based implementation of token storage.
type FileTokenStore struct {
	Path string
}

// NewFileTokenStore creates a new FileTokenStore with the given path.
func NewFileTokenStore(path string) *FileTokenStore {
	return &FileTokenStore{Path: path}
}

// SaveToken saves an OAuth token to the file at store.Path.
func (store *FileTokenStore) SaveToken(token *oauth2.Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	if err := os.WriteFile(store.Path, data, 0600); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}

	return nil
}

// LoadToken loads an OAuth token from the file at store.Path.
// Returns nil, nil if the file does not exist (no error).
func (store *FileTokenStore) LoadToken() (*oauth2.Token, error) {
	data, err := os.ReadFile(store.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read token file: %w", err)
	}

	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	return &token, nil
}

