package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// LoadCredsFile parses a Google Cloud Console "credentials.json" for a
// Desktop OAuth client. The format is:
//
//	{"installed":{"client_id":"...","client_secret":"...", ... }}
func LoadCredsFile(path string) (ClientCreds, error) {
	b, err := os.ReadFile(path) //nolint:gosec // user-supplied path
	if err != nil {
		return ClientCreds{}, fmt.Errorf("read %s: %w", path, err)
	}
	var raw struct {
		Installed struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		} `json:"installed"`
		Web struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		} `json:"web"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return ClientCreds{}, fmt.Errorf("parse %s: %w", path, err)
	}
	c := ClientCreds{ClientID: raw.Installed.ClientID, ClientSecret: raw.Installed.ClientSecret}
	if c.ClientID == "" {
		c = ClientCreds{ClientID: raw.Web.ClientID, ClientSecret: raw.Web.ClientSecret}
	}
	if c.ClientID == "" {
		return ClientCreds{}, errors.New("credentials file has no client_id under 'installed' or 'web'")
	}
	return c, nil
}

// Revoke calls Google's token-revocation endpoint with the refresh token.
// Best-effort: a 200 or 400 (already-revoked) is treated as success.
func Revoke(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(refreshToken) == "" {
		return nil
	}
	body := strings.NewReader(url.Values{"token": {refreshToken}}.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://oauth2.googleapis.com/revoke", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("revoke: server returned %d", resp.StatusCode)
	}
	return nil
}
