// Package money handles USD amounts as int64 cents. No float64 ever.
package money

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrParse is returned when an amount string fails to parse.
var ErrParse = errors.New("invalid usd amount")

// ParseDollar parses a USD amount string into int64 cents.
//
// Accepted grammar:
//
//	$1,402.50   -> 140250  ($-prefixed allows comma thousand separators)
//	$1402       -> 140200  ($ alone, integer dollars)
//	1402.50     -> 140250  (no $, no commas, decimal allowed)
//	1402        -> 140200  (no $, integer = whole dollars)
//	0           ->      0
//
// Rejected:
//
//	1,402.50    (commas without $ — ambiguous with EU)
//	$1.402,50   (EU-style)
//	1402.5      (single decimal digit not allowed; require 0 or 2)
//	-5          (negative)
//	$.50        (leading dot without integer part)
func ParseDollar(s string) (int64, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return 0, fmt.Errorf("%w: empty", ErrParse)
	}

	hasDollar := strings.HasPrefix(raw, "$")
	body := strings.TrimPrefix(raw, "$")
	if body == "" {
		return 0, fmt.Errorf("%w: %q", ErrParse, s)
	}
	if strings.HasPrefix(body, "-") || strings.HasPrefix(body, "+") {
		return 0, fmt.Errorf("%w: signed amounts not allowed: %q", ErrParse, s)
	}

	if strings.Contains(body, ",") {
		if !hasDollar {
			return 0, fmt.Errorf("%w: commas only allowed with $ prefix: %q", ErrParse, s)
		}
		body = strings.ReplaceAll(body, ",", "")
	}

	intPart, fracPart, hasFrac := strings.Cut(body, ".")
	if intPart == "" {
		return 0, fmt.Errorf("%w: missing integer part: %q", ErrParse, s)
	}

	dollars, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil || dollars < 0 {
		return 0, fmt.Errorf("%w: bad integer part %q: %v", ErrParse, intPart, err)
	}

	var cents int64
	if hasFrac {
		if len(fracPart) != 2 {
			return 0, fmt.Errorf("%w: fractional part must be exactly 2 digits: %q", ErrParse, s)
		}
		c, err := strconv.ParseInt(fracPart, 10, 64)
		if err != nil || c < 0 {
			return 0, fmt.Errorf("%w: bad fractional part %q", ErrParse, fracPart)
		}
		cents = c
	}

	if dollars > (1<<62)/100 {
		return 0, fmt.Errorf("%w: amount overflow: %q", ErrParse, s)
	}
	return dollars*100 + cents, nil
}

// FormatUSD formats int64 cents as a US-locale dollar string (e.g. "$1,402.50").
// Always 2 decimal places. Negative values prefix the minus before the dollar sign.
func FormatUSD(cents int64) string {
	negative := cents < 0
	if negative {
		cents = -cents
	}
	dollars := cents / 100
	frac := cents % 100

	intStr := strconv.FormatInt(dollars, 10)
	intStr = addThousandSeparators(intStr)

	sign := ""
	if negative {
		sign = "-"
	}
	return fmt.Sprintf("%s$%s.%02d", sign, intStr, frac)
}

func addThousandSeparators(s string) string {
	n := len(s)
	if n <= 3 {
		return s
	}
	first := n % 3
	var b strings.Builder
	b.Grow(n + n/3)
	if first > 0 {
		b.WriteString(s[:first])
		if n > first {
			b.WriteByte(',')
		}
	}
	for i := first; i < n; i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < n {
			b.WriteByte(',')
		}
	}
	return b.String()
}
