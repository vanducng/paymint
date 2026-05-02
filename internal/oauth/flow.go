package oauth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/oauth2"
)

// Scopes paymint needs.
const (
	ScopeDriveFile     = "https://www.googleapis.com/auth/drive.file"
	ScopeDriveMetadata = "https://www.googleapis.com/auth/drive.metadata.readonly"
)

// DefaultScopes returns the minimum scope set per Red Team F6.
func DefaultScopes() []string {
	return []string{ScopeDriveFile, ScopeDriveMetadata}
}

// ClientCreds holds the static client identifiers from a Google Cloud OAuth
// "Desktop" client. With PKCE the secret is optional; we accept it because
// google.ConfigFromJSON returns one and Google's token endpoint accepts it.
type ClientCreds struct {
	ClientID     string
	ClientSecret string
}

// LoginOptions tunes the desktop flow. Zero values pick safe defaults.
type LoginOptions struct {
	Scopes  []string
	Timeout time.Duration // overall deadline on the login flow
	OpenURL func(string) error
	OnAuth  func(authURL string) // called with auth URL after listener is up
}

// Login runs the desktop loopback flow with PKCE+state and returns a token.
//
// Flow:
//  1. Bind a TCP listener on 127.0.0.1 (DNS rebinding-safe, never localhost).
//  2. Generate PKCE verifier / challenge and a state CSRF.
//  3. Build the authorisation URL and open it in the browser (or print it).
//  4. Wait for exactly one callback to /callback; verify state, exchange
//     code for token using the verifier. The listener stops after the first
//     valid callback (no spinner left running).
//  5. Return the token. Caller persists via SaveToken.
func Login(ctx context.Context, creds ClientCreds, opts LoginOptions) (*oauth2.Token, error) {
	if opts.Scopes == nil {
		opts.Scopes = DefaultScopes()
	}
	if opts.Timeout == 0 {
		opts.Timeout = 2 * time.Minute
	}

	verifier, err := GenerateVerifier()
	if err != nil {
		return nil, err
	}
	state, err := GenerateState()
	if err != nil {
		return nil, err
	}
	challenge := S256Challenge(verifier)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("loopback listen: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	cfg := &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Scopes:       opts.Scopes,
		RedirectURL:  redirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
	}
	authURL := cfg.AuthCodeURL(state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("prompt", "consent"))

	if opts.OnAuth != nil {
		opts.OnAuth(authURL)
	}
	if opts.OpenURL != nil {
		_ = opts.OpenURL(authURL) // best-effort; user can also paste the URL
	}

	resCh := make(chan result, 1)

	srv := &http.Server{
		Handler:           callbackHandler(cfg, verifier, state, resCh),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() { _ = srv.Serve(listener) }()

	deadline := time.NewTimer(opts.Timeout)
	defer deadline.Stop()

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	select {
	case r := <-resCh:
		return r.tok, r.err
	case <-deadline.C:
		return nil, errors.New("login timed out (no callback within 2 minutes)")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// callbackHandler validates state and exchanges the code, sending the result
// onto ch exactly once.
func callbackHandler(cfg *oauth2.Config, verifier, expectedState string, ch chan<- result) http.Handler {
	var handled bool
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		if handled {
			http.Error(w, "callback already handled", http.StatusConflict)
			return
		}
		handled = true

		q := r.URL.Query()
		if errStr := q.Get("error"); errStr != "" {
			send(ch, result{err: fmt.Errorf("oauth error: %s (%s)", errStr, q.Get("error_description"))})
			writeFailure(w, errStr)
			return
		}
		if got := q.Get("state"); got != expectedState {
			send(ch, result{err: fmt.Errorf("state mismatch: csrf protection triggered")})
			writeFailure(w, "state mismatch")
			return
		}
		code := q.Get("code")
		if code == "" {
			send(ch, result{err: errors.New("no code in callback")})
			writeFailure(w, "no code")
			return
		}
		tok, err := cfg.Exchange(r.Context(), code,
			oauth2.SetAuthURLParam("code_verifier", verifier))
		if err != nil {
			send(ch, result{err: fmt.Errorf("token exchange: %w", err)})
			writeFailure(w, "token exchange failed")
			return
		}
		writeSuccess(w)
		send(ch, result{tok: tok})
	})
}

type result struct {
	tok *oauth2.Token
	err error
}

func send(ch chan<- result, r result) {
	select {
	case ch <- r:
	default: // channel already full — login completed via another path
	}
}

// successPage / failurePage are static — no user-controlled HTML, no scripts.
const successPage = `<!DOCTYPE html>
<html><head><title>paymint — login complete</title></head>
<body style="font-family:sans-serif;max-width:520px;margin:6em auto;text-align:center">
<h1>You're signed in.</h1>
<p>You can close this tab and return to the terminal.</p>
</body></html>`

func writeSuccess(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(successPage))
}

func writeFailure(w http.ResponseWriter, reason string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	// reason is constrained to OAuth-defined strings; still escape via url.PathEscape
	// to be safe (defence in depth).
	_, _ = fmt.Fprintf(w,
		`<!DOCTYPE html><html><body style="font-family:sans-serif;max-width:520px;margin:6em auto;text-align:center">`+
			`<h1>Login failed</h1><p>Reason: %s</p><p>Return to the terminal for details.</p></body></html>`,
		url.PathEscape(reason))
}
