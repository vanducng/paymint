package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	yaml "github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"

	"github.com/vanducng/paymint/internal/core/config"
	"github.com/vanducng/paymint/internal/drive"
	"github.com/vanducng/paymint/internal/oauth"
	"github.com/vanducng/paymint/internal/sheets"
	"github.com/vanducng/paymint/internal/sync"
)

func newSyncCmd() *cobra.Command {
	var credsPath string
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Round-trip the data dir against the configured Google Sheet.",
		Long: "Pulls all 5 tabs into local YAML, pushes pending ops back to the sheet,\n" +
			"verifies Drive's revision counter is unchanged, and snapshots the data\n" +
			"dir to git (if it's a git repo).",
		RunE: runSync(&credsPath),
	}
	cmd.Flags().StringVar(&credsPath, "credentials", "",
		"path to credentials.json (overrides oauth_client_id in config)")
	return cmd
}

func runSync(credsPath *string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		// We don't hold the data-dir lock here because sync.Run takes its own.
		// Resolve files separately:
		root, err := resolveDataDir(cmd)
		if err != nil {
			return err
		}
		files, err := newDataDirFiles(root)
		if err != nil {
			return err
		}
		if err := files.requireInitialized(); err != nil {
			return err
		}

		cfg, err := loadCfg(files.configPath)
		if err != nil {
			return err
		}
		if cfg.SheetID == "" {
			return errors.New("config: sheet_id is empty (edit .paymint/config.yaml)")
		}

		creds, err := resolveCreds(*credsPath, cfg)
		if err != nil {
			return err
		}
		tokenPath, err := oauth.TokenPath()
		if err != nil {
			return err
		}
		tok, err := oauth.LoadToken(tokenPath)
		if err != nil {
			return err
		}
		if tok == nil {
			return errors.New("not signed in — run 'paymint auth login --credentials <path>' first")
		}

		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
		defer cancel()
		httpClient := buildOAuthClient(ctx, creds, tok, tokenPath)

		sheetsClient, err := sheets.NewService(ctx, httpClient)
		if err != nil {
			return err
		}
		driveClient, err := drive.NewService(ctx, httpClient)
		if err != nil {
			return err
		}

		res, err := sync.Run(ctx, sync.Config{
			SpreadsheetID: cfg.SheetID,
			DataDir:       files.root,
			YAMLPaths:     files.yamlPaths,
			LockPath:      files.lockPath,
			PendingPath:   files.pendingPath,
			Sheets:        sheetsClient,
			Drive:         driveClient,
			Logger:        cmd.OutOrStdout(),
		})
		if err != nil {
			if res != nil {
				cmd.Printf("partial: pulled=%d, pushed=%d\n", res.PulledRows, res.PushedOps)
			}
			return err
		}
		cmd.Printf("sync ok: pulled=%d, pushed=%d, retries=%d, commit=%v\n",
			res.PulledRows, res.PushedOps, res.Retries, res.CommitMade)
		return nil
	}
}

// loadCfg reads .paymint/config.yaml into a Config.
func loadCfg(path string) (*config.Config, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path validated upstream
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c config.Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &c, nil
}

// resolveCreds returns ClientCreds from a credentials.json path or, failing
// that, from the OAuth client ID stored in config.
func resolveCreds(credsPath string, cfg *config.Config) (oauth.ClientCreds, error) {
	if credsPath != "" {
		return oauth.LoadCredsFile(credsPath)
	}
	if cfg.ClientID == "" {
		return oauth.ClientCreds{}, errors.New(
			"no OAuth client: pass --credentials <file> or set oauth_client_id in config.yaml")
	}
	return oauth.ClientCreds{ClientID: cfg.ClientID}, nil
}

// buildOAuthClient produces an authorised *http.Client whose token source
// persists refreshed tokens back to disk under flock (Red Team F12).
func buildOAuthClient(ctx context.Context, creds oauth.ClientCreds, tok *oauth2.Token, tokenPath string) *http.Client {
	cfg := &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
		Scopes: oauth.DefaultScopes(),
	}
	src := &persistingTokenSource{
		base:      cfg.TokenSource(ctx, tok),
		tokenPath: tokenPath,
		current:   tok,
	}
	return oauth2.NewClient(ctx, src)
}

// persistingTokenSource wraps an oauth2.TokenSource; on refresh it persists
// the new token to disk under a file lock so concurrent paymint processes
// (cron + interactive) don't both try to use a single-use refresh token.
type persistingTokenSource struct {
	base      oauth2.TokenSource
	tokenPath string
	current   *oauth2.Token
}

// Token returns the latest token, persisting any refreshed value.
func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.base.Token()
	if err != nil {
		return nil, err
	}
	if p.current == nil || tok.AccessToken != p.current.AccessToken {
		if err := oauth.WithLockedToken(p.tokenPath, func() error {
			return oauth.SaveToken(p.tokenPath, tok)
		}); err != nil {
			return nil, fmt.Errorf("persist refreshed token: %w", err)
		}
		p.current = tok
	}
	return tok, nil
}
