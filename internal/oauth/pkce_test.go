package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateVerifier_LengthsAndUniqueness(t *testing.T) {
	v1, err := GenerateVerifier()
	require.NoError(t, err)
	v2, err := GenerateVerifier()
	require.NoError(t, err)

	assert.NoError(t, ValidateVerifier(v1))
	assert.NoError(t, ValidateVerifier(v2))
	assert.NotEqual(t, v1, v2)
}

func TestS256Challenge_MatchesSpec(t *testing.T) {
	v := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	want := base64.RawURLEncoding.EncodeToString(func() []byte { h := sha256.Sum256([]byte(v)); return h[:] }())
	assert.Equal(t, want, S256Challenge(v))
}

func TestGenerateState_NotEqual(t *testing.T) {
	s1, err := GenerateState()
	require.NoError(t, err)
	s2, err := GenerateState()
	require.NoError(t, err)
	assert.NotEqual(t, s1, s2)
	// raw URL encoding of 32 bytes = 43 chars (no padding).
	assert.Equal(t, 43, len(s1))
}
