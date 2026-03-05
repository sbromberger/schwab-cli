package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Token holds the OAuth tokens and their expiry.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// IsExpired reports whether the access token is expired or within 60 seconds of expiring.
func (t *Token) IsExpired() bool {
	return time.Now().After(t.ExpiresAt.Add(-60 * time.Second))
}

// tokenPath returns the path to token.json.
func tokenPath(configDir string) string {
	return filepath.Join(configDir, "token.json")
}

// LoadToken reads token.json from the config directory.
// Returns os.ErrNotExist if the file does not exist.
func LoadToken(configDir string) (*Token, error) {
	path := tokenPath(configDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Token
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("malformed token file %s: %w", path, err)
	}
	return &t, nil
}

// SaveToken writes the token to token.json with 0600 permissions.
func SaveToken(configDir string, t *Token) error {
	path := tokenPath(configDir)
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal token: %w", err)
	}
	// Write via temp file + rename for atomicity.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("cannot write token file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("cannot save token file: %w", err)
	}
	return nil
}
