package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateID_Accepted(t *testing.T) {
	for _, id := range []string{
		"abs",
		"acme-corp",
		"a",
		"a1",
		"a-b-c",
		"abc123",
		strings.Repeat("a", 64),
	} {
		assert.NoErrorf(t, ValidateID(id), "id=%q", id)
	}
}

func TestValidateID_Rejected(t *testing.T) {
	for _, id := range []string{
		"",
		"-abs", // leading dash
		"abs-", // trailing dash
		"ABS",  // uppercase
		"a/b",
		"a\\b",
		"..",
		"a..b",
		"a b",
		"a\x00b",
		"a\nb",
		"con", // reserved
		"NUL", // reserved (case insensitive)
		strings.Repeat("a", 65),
	} {
		assert.Errorf(t, ValidateID(id), "expected reject for %q", id)
	}
}

func TestValidateInvoiceID(t *testing.T) {
	slug, year, month, err := ValidateInvoiceID("INV-abs-202604")
	require.NoError(t, err)
	assert.Equal(t, "abs", slug)
	assert.Equal(t, 2026, year)
	assert.Equal(t, 4, month)

	for _, id := range []string{
		"",
		"INV-abs",
		"INV-abs-2026",
		"INV-abs-202613", // bad month
		"inv-abs-202604", // lowercase prefix
		"INV-ABS-202604", // uppercase slug
		"INV--202604",
		"INV-abs-20260a",
	} {
		_, _, _, err := ValidateInvoiceID(id)
		assert.Errorf(t, err, "expected reject for %q", id)
	}
}

func TestMakeInvoiceID(t *testing.T) {
	assert.Equal(t, "INV-abs-202604", MakeInvoiceID("abs", 2026, 4))
	assert.Equal(t, "INV-acme-corp-202612", MakeInvoiceID("acme-corp", 2026, 12))
}
