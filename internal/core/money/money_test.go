package money

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDollar_Accepted(t *testing.T) {
	cases := map[string]int64{
		"0":         0,
		"1":         100,
		"1.00":      100,
		"1.50":      150,
		"$1":        100,
		"$1.00":     100,
		"$85.00":    8500,
		"85.00":     8500,
		"85":        8500,
		"$1,402.50": 140250,
		"1402.50":   140250,
		"1402":      140200,
		"  $5.00 ":  500,
	}
	for in, want := range cases {
		got, err := ParseDollar(in)
		require.NoErrorf(t, err, "ParseDollar(%q)", in)
		assert.Equalf(t, want, got, "ParseDollar(%q)", in)
	}
}

func TestParseDollar_Rejected(t *testing.T) {
	cases := []string{
		"",
		"abc",
		"-5",
		"+5",
		"$",
		"$.50",
		"1,402.50",  // commas without $
		"$1.402,50", // EU style
		"1.5",       // single decimal digit
		"$5.123",    // 3 decimals
		"$5..00",
		"$$5",
	}
	for _, in := range cases {
		_, err := ParseDollar(in)
		assert.Errorf(t, err, "expected error for %q", in)
	}
}

func TestFormatUSD(t *testing.T) {
	cases := map[int64]string{
		0:       "$0.00",
		1:       "$0.01",
		100:     "$1.00",
		8500:    "$85.00",
		140250:  "$1,402.50",
		1234567: "$12,345.67",
		-100:    "-$1.00",
		-140250: "-$1,402.50",
	}
	for in, want := range cases {
		assert.Equalf(t, want, FormatUSD(in), "FormatUSD(%d)", in)
	}
}

func TestParseDollar_FormatUSD_Roundtrip(t *testing.T) {
	for _, cents := range []int64{0, 1, 100, 8500, 140250, 999999999} {
		s := FormatUSD(cents)
		got, err := ParseDollar(s)
		require.NoErrorf(t, err, "round-trip %d -> %q", cents, s)
		assert.Equal(t, cents, got)
	}
}
