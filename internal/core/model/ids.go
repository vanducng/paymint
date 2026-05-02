package model

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// idRegex enforces lowercase kebab IDs. Rejects path traversal, NUL,
// control chars, Windows-reserved tokens — anything that could escape a
// filename or surprise a shell. See red-team finding F7.
var idRegex = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

// invoiceIDRegex matches INV-<slug>-<YYYYMM>. The slug part is 1-40 chars of
// the same kebab grammar enforced by idRegex.
var invoiceIDRegex = regexp.MustCompile(`^INV-([a-z0-9](?:[a-z0-9-]{0,38}[a-z0-9])?)-(\d{4})(\d{2})$`)

// reservedNames are Windows-reserved filenames. Even on macOS/Linux we reject
// them so a future cross-platform port doesn't break.
var reservedNames = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true,
	"com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true,
	"lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// ErrInvalidID is returned when an ID fails validation.
var ErrInvalidID = errors.New("invalid id")

// ValidateID asserts an entity ID is safe to use as part of a filename.
func ValidateID(id string) error {
	if id == "" {
		return fmt.Errorf("%w: empty", ErrInvalidID)
	}
	if !idRegex.MatchString(id) {
		return fmt.Errorf("%w: %q (must match %s)", ErrInvalidID, id, idRegex.String())
	}
	if reservedNames[strings.ToLower(id)] {
		return fmt.Errorf("%w: %q is reserved", ErrInvalidID, id)
	}
	return nil
}

// ValidateInvoiceID asserts an invoice ID matches `INV-<slug>-<YYYYMM>` and
// that the embedded year-month parses. Returns the slug and the year/month.
func ValidateInvoiceID(id string) (slug string, year, month int, err error) {
	m := invoiceIDRegex.FindStringSubmatch(id)
	if m == nil {
		return "", 0, 0, fmt.Errorf("%w: %q (want INV-<slug>-<YYYYMM>)", ErrInvalidID, id)
	}
	slug = m[1]
	if reservedNames[slug] {
		return "", 0, 0, fmt.Errorf("%w: slug %q is reserved", ErrInvalidID, slug)
	}
	// Regex already constrained both groups to digits; Atoi cannot fail.
	year, _ = strconv.Atoi(m[2])
	month, _ = strconv.Atoi(m[3])
	if month < 1 || month > 12 {
		return "", 0, 0, fmt.Errorf("%w: month %02d out of range", ErrInvalidID, month)
	}
	return slug, year, month, nil
}

// MakeInvoiceID returns the canonical ID for a (slug, year, month) tuple.
// Caller is responsible for validating the slug separately.
func MakeInvoiceID(slug string, year, month int) string {
	return fmt.Sprintf("INV-%s-%04d%02d", slug, year, month)
}
