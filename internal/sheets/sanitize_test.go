package sheets

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeCell_FormulaPrefix(t *testing.T) {
	for _, in := range []string{"=SUM(A1:A2)", "+evil", "-1", "@cmd", "\tlead", "\rlead"} {
		got, err := SanitizeCell(in)
		require.NoErrorf(t, err, "input %q", in)
		assert.Truef(t, strings.HasPrefix(got, "'"),
			"expected leading quote on %q, got %q", in, got)
	}
}

func TestSanitizeCell_PassThrough(t *testing.T) {
	for _, in := range []string{"hello world", "abs", "1402.50", "Adventure Bound Studio"} {
		got, err := SanitizeCell(in)
		require.NoError(t, err)
		assert.Equal(t, in, got)
	}
}

func TestSanitizeCell_StripsControlChars(t *testing.T) {
	got, err := SanitizeCell("foo\x07bar")
	require.NoError(t, err)
	assert.Equal(t, "foobar", got)
}

func TestSanitizeCell_RejectsOversized(t *testing.T) {
	_, err := SanitizeCell(strings.Repeat("x", MaxStringLen+1))
	assert.Error(t, err)
}

func TestSanitizeRow_Mixed(t *testing.T) {
	row := []any{"abs", "=evil", 8500, nil}
	out, err := SanitizeRow(row)
	require.NoError(t, err)
	assert.Equal(t, "abs", out[0])
	assert.Equal(t, "'=evil", out[1])
	assert.Equal(t, "8500", out[2])
	assert.Equal(t, "", out[3])
}
