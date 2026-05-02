package oauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"

	"github.com/vanducng/paymint/internal/store/locks"
)

// TokenPath returns the on-disk location of paymint's OAuth token. Honours
// $XDG_CONFIG_HOME → falls back to os.UserConfigDir → "~/.config" lastditch.
func TokenPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "paymint", "token.json"), nil
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(cfg, "paymint", "token.json"), nil
}

// LoadToken reads a previously persisted token. Returns (nil, nil) if the
// token file doesn't exist (caller treats as "logged out").
func LoadToken(path string) (*oauth2.Token, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path is platform user config
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var t oauth2.Token
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &t, nil
}

// SaveToken writes the token atomically (mode 0600).
func SaveToken(path string, t *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// DeleteToken removes the persisted token. Missing-file is not an error.
func DeleteToken(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// WithLockedToken takes a token-file lock for the duration of fn — used
// during refresh to prevent two paymint processes from both invalidating a
// single-use refresh token (Red Team F12). Lock is sibling to the token at
// "<path>.lock" so it persists independently.
func WithLockedToken(path string, fn func() error) error {
	lockPath := path + ".lock"
	unlock, err := locks.Acquire(lockPath, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = unlock() }()
	return fn()
}
