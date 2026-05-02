package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/vanducng/paymint/internal/oauth"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Manage Google OAuth credentials."}
	cmd.AddCommand(newAuthLoginCmd(), newAuthStatusCmd(), newAuthLogoutCmd())
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var credsPath string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in to Google with PKCE — stores token at the user-config path.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			creds, err := oauth.LoadCredsFile(credsPath)
			if err != nil {
				return err
			}
			tokenPath, err := oauth.TokenPath()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Minute)
			defer cancel()

			out := cmd.OutOrStdout()
			tok, err := oauth.Login(ctx, creds, oauth.LoginOptions{
				OnAuth: func(authURL string) {
					_, _ = fmt.Fprintf(out,
						"Open this URL to authorise paymint:\n  %s\nWaiting for callback...\n",
						authURL)
				},
				OpenURL: openInBrowser,
			})
			if err != nil {
				return err
			}
			if err := oauth.WithLockedToken(tokenPath, func() error {
				return oauth.SaveToken(tokenPath, tok)
			}); err != nil {
				return err
			}
			cmd.Printf("Signed in. Token cached at %s\n", tokenPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&credsPath, "credentials", "",
		"path to a Google Cloud credentials.json (Desktop OAuth client; required)")
	_ = cmd.MarkFlagRequired("credentials")
	return cmd
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print the current OAuth token state.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			tokenPath, err := oauth.TokenPath()
			if err != nil {
				return err
			}
			tok, err := oauth.LoadToken(tokenPath)
			if err != nil {
				return err
			}
			if tok == nil {
				cmd.Println("not signed in (run 'paymint auth login --credentials <path>')")
				return nil
			}
			expires := "no expiry"
			if !tok.Expiry.IsZero() {
				expires = tok.Expiry.Local().Format(time.RFC3339)
				if tok.Expiry.Before(time.Now()) {
					expires += " (EXPIRED — refresh runs automatically on next sync)"
				}
			}
			cmd.Printf("token: %s\nexpiry: %s\nrefresh-token: %s\n",
				tokenPath, expires, hasOrNo(tok.RefreshToken != ""))
			return nil
		},
	}
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke the refresh token (best-effort) and delete the local token file.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			tokenPath, err := oauth.TokenPath()
			if err != nil {
				return err
			}
			tok, err := oauth.LoadToken(tokenPath)
			if err != nil {
				return err
			}
			if tok == nil {
				cmd.Println("nothing to do — no cached token")
				return nil
			}
			if err := oauth.Revoke(cmd.Context(), tok.RefreshToken); err != nil {
				cmd.Printf("warning: revoke failed (%v) — proceeding to delete local token\n", err)
			}
			if err := oauth.WithLockedToken(tokenPath, func() error {
				return oauth.DeleteToken(tokenPath)
			}); err != nil {
				return err
			}
			cmd.Println("signed out.")
			return nil
		},
	}
}

func hasOrNo(b bool) string {
	if b {
		return "present"
	}
	return "missing"
}

// openInBrowser tries common per-OS launchers. Best-effort — caller prints the
// URL anyway so the user can paste it manually.
func openInBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url) //nolint:gosec // url is known auth URL we just generated
	case "linux":
		cmd = exec.Command("xdg-open", url) //nolint:gosec
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url) //nolint:gosec
	default:
		return errors.New("unsupported platform")
	}
	return cmd.Start()
}
