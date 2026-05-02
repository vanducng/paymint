package period

import (
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	ym, err := Parse("2026-04")
	require.NoError(t, err)
	assert.Equal(t, YearMonth{Year: 2026, Month: time.April}, ym)

	for _, bad := range []string{"", "2026", "2026-04-01", "2026/04", "abc-04", "2026-13", "2026-00"} {
		_, err := Parse(bad)
		assert.Errorf(t, err, "expected reject %q", bad)
	}
}

func TestYearMonth_Comparison(t *testing.T) {
	a := YearMonth{Year: 2026, Month: time.April}
	b := YearMonth{Year: 2026, Month: time.May}
	c := YearMonth{Year: 2027, Month: time.January}

	assert.True(t, a.Before(b))
	assert.True(t, b.Before(c))
	assert.True(t, c.After(a))
	assert.False(t, a.Before(a))
}

func TestYearMonth_Next(t *testing.T) {
	assert.Equal(t,
		YearMonth{Year: 2027, Month: time.January},
		YearMonth{Year: 2026, Month: time.December}.Next())
	assert.Equal(t,
		YearMonth{Year: 2026, Month: time.May},
		YearMonth{Year: 2026, Month: time.April}.Next())
}

func TestRange_Months(t *testing.T) {
	r := Range{
		Start: YearMonth{Year: 2026, Month: time.January},
		End:   YearMonth{Year: 2026, Month: time.March},
	}
	var months []YearMonth
	for ym := range r.Months() {
		months = append(months, ym)
	}
	assert.Equal(t, []YearMonth{
		{Year: 2026, Month: time.January},
		{Year: 2026, Month: time.February},
		{Year: 2026, Month: time.March},
	}, months)
}

func TestRange_Contains(t *testing.T) {
	r := Range{
		Start: YearMonth{Year: 2026, Month: time.January},
		End:   YearMonth{Year: 2026, Month: time.March},
	}
	assert.True(t, r.Contains(YearMonth{Year: 2026, Month: time.January}))
	assert.True(t, r.Contains(YearMonth{Year: 2026, Month: time.March}))
	assert.False(t, r.Contains(YearMonth{Year: 2025, Month: time.December}))
	assert.False(t, r.Contains(YearMonth{Year: 2026, Month: time.April}))
}

func TestFromDate(t *testing.T) {
	d := civil.Date{Year: 2026, Month: time.April, Day: 17}
	assert.Equal(t, YearMonth{Year: 2026, Month: time.April}, FromDate(d))
}
