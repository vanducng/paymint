// Package period provides calendar year-month helpers used throughout the
// ledger. No timezones, no clocks — civil dates and discrete months only.
package period

import (
	"errors"
	"fmt"
	"iter"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/civil"
)

// ErrParse is returned when a YearMonth string fails to parse.
var ErrParse = errors.New("invalid year-month")

// YearMonth identifies a calendar month, e.g. April 2026.
type YearMonth struct {
	Year  int
	Month time.Month
}

// Parse accepts "YYYY-MM" (e.g. "2026-04").
func Parse(s string) (YearMonth, error) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 || len(parts[0]) != 4 || len(parts[1]) != 2 {
		return YearMonth{}, fmt.Errorf("%w: %q (want YYYY-MM)", ErrParse, s)
	}
	y, err := strconv.Atoi(parts[0])
	if err != nil || y < 1 {
		return YearMonth{}, fmt.Errorf("%w: bad year %q", ErrParse, parts[0])
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 1 || m > 12 {
		return YearMonth{}, fmt.Errorf("%w: bad month %q", ErrParse, parts[1])
	}
	return YearMonth{Year: y, Month: time.Month(m)}, nil
}

// FromDate returns the YearMonth containing the given civil.Date.
func FromDate(d civil.Date) YearMonth {
	return YearMonth{Year: d.Year, Month: d.Month}
}

// String returns the canonical "YYYY-MM" form.
func (ym YearMonth) String() string {
	return fmt.Sprintf("%04d-%02d", ym.Year, ym.Month)
}

// Before reports whether ym is strictly before other.
func (ym YearMonth) Before(other YearMonth) bool {
	if ym.Year != other.Year {
		return ym.Year < other.Year
	}
	return ym.Month < other.Month
}

// After reports whether ym is strictly after other.
func (ym YearMonth) After(other YearMonth) bool { return other.Before(ym) }

// Next returns the month following ym.
func (ym YearMonth) Next() YearMonth {
	if ym.Month == time.December {
		return YearMonth{Year: ym.Year + 1, Month: time.January}
	}
	return YearMonth{Year: ym.Year, Month: ym.Month + 1}
}

// Range is an inclusive [Start, End] window of months.
type Range struct {
	Start, End YearMonth
}

// Contains reports whether ym falls within r (inclusive on both ends).
func (r Range) Contains(ym YearMonth) bool {
	return !ym.Before(r.Start) && !ym.After(r.End)
}

// Months yields each YearMonth in r from Start to End inclusive.
func (r Range) Months() iter.Seq[YearMonth] {
	return func(yield func(YearMonth) bool) {
		cur := r.Start
		for !cur.After(r.End) {
			if !yield(cur) {
				return
			}
			cur = cur.Next()
		}
	}
}
