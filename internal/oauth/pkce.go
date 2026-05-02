// Package oauth implements the desktop-loopback OAuth 2.0 flow with PKCE
// (RFC 7636) for paymint. PKCE eliminates the need for an embedded client
// secret on desktop clients (Red Team F1).
package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

// MinVerifierLen is the RFC 7636 lower bound (43 chars after base64url).
const MinVerifierLen = 43

// MaxVerifierLen is the RFC 7636 upper bound (128 chars).
const MaxVerifierLen = 128

// GenerateVerifier returns a cryptographically random 64-byte PKCE verifier
// (the encoded form lands at 86 chars, well within the 43-128 spec window).
func GenerateVerifier() (string, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// S256Challenge returns the BASE64URL(SHA-256(verifier)) challenge string.
func S256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// ValidateVerifier asserts the verifier sits inside RFC 7636's length window.
// Used by tests; production code generates verifiers via GenerateVerifier and
// they're always in range.
func ValidateVerifier(v string) error {
	if len(v) < MinVerifierLen || len(v) > MaxVerifierLen {
		return errors.New("pkce verifier outside [43, 128] characters")
	}
	return nil
}

// GenerateState returns 32 random bytes base64url-encoded — the OAuth `state`
// CSRF parameter. The redirect handler must compare strings.
func GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
