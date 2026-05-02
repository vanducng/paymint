package sheets

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// MaxStringLen caps any single cell on read. Defends against pathological
// rows pulled from a hand-edited sheet.
const MaxStringLen = 4000

// formulaPrefixes are leading characters that Sheets / Excel interpret as
// the start of a formula. We prefix a single quote to neutralise them on
// pull (Red Team F8). Push side never sends USER_ENTERED, so this is
// belt-and-suspenders.
var formulaPrefixes = "=+-@\t\r"

// SanitizeCell returns s with control characters stripped and any leading
// formula-trigger character escaped. Returns an error if the result would
// exceed MaxStringLen — pulling such a cell aborts the sync rather than
// silently truncating.
func SanitizeCell(s string) (string, error) {
	if len(s) > MaxStringLen {
		return "", fmt.Errorf("cell exceeds %d chars (got %d)", MaxStringLen, len(s))
	}
	cleaned := stripControl(s)
	if cleaned == "" {
		return cleaned, nil
	}
	if strings.ContainsRune(formulaPrefixes, rune(cleaned[0])) {
		cleaned = "'" + cleaned
	}
	return cleaned, nil
}

// SanitizeRow applies SanitizeCell to every cell in the row. The first
// error short-circuits — caller surfaces sheet-row context to the user.
func SanitizeRow(row []any) ([]any, error) {
	out := make([]any, len(row))
	for i, v := range row {
		switch t := v.(type) {
		case string:
			s, err := SanitizeCell(t)
			if err != nil {
				return nil, fmt.Errorf("col %d: %w", i, err)
			}
			out[i] = s
		case nil:
			out[i] = ""
		default:
			out[i] = fmt.Sprint(v)
			s, err := SanitizeCell(out[i].(string))
			if err != nil {
				return nil, fmt.Errorf("col %d: %w", i, err)
			}
			out[i] = s
		}
	}
	return out, nil
}

func stripControl(s string) string {
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !isControl(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isControl(r rune) bool {
	// Allow tab and newline only inside cell bodies if they survive
	// SanitizeCell — but tab/CR are formula triggers when leading and
	// otherwise harmless. Keep it conservative: drop true control chars.
	if r == '\n' || r == '\t' || r == '\r' {
		return false
	}
	return unicode.IsControl(r)
}

// ErrEmpty is returned by row decoders when a required column is blank.
var ErrEmpty = errors.New("required cell empty")

// FieldString returns row[i] as a string, blanking non-strings safely.
func FieldString(row []any, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	if s, ok := row[i].(string); ok {
		return s
	}
	return fmt.Sprint(row[i])
}
