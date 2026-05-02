// Package output renders structured CLI results — tables for human eyes,
// JSON deferred to v0.2.
package output

import (
	"io"

	"github.com/jedib0t/go-pretty/v6/table"
)

// PrintTable writes a simple-styled, ASCII-only table.
func PrintTable(w io.Writer, header []any, rows [][]any) {
	t := table.NewWriter()
	t.SetOutputMirror(w)
	t.AppendHeader(table.Row(header))
	for _, r := range rows {
		t.AppendRow(table.Row(r))
	}
	t.SetStyle(table.StyleLight)
	t.Render()
}
