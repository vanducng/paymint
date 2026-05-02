// Package pdf turns a pdfdoc.Statement into bytes via maroto v2. All maroto
// API calls are confined here so version churn doesn't leak into core.
package pdf

import (
	"fmt"
	"io"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/consts/pagesize"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"

	"github.com/vanducng/paymint/internal/core/money"
	"github.com/vanducng/paymint/internal/core/pdfdoc"
)

// Render writes a Statement as a PDF to w.
func Render(st *pdfdoc.Statement, w io.Writer) error {
	cfg := config.NewBuilder().
		WithPageSize(pagesize.A4).
		WithLeftMargin(15).WithRightMargin(15).WithTopMargin(15).
		Build()
	m := maroto.New(cfg)

	header(m, st)
	addressBlock(m, st)
	invoiceMeta(m, st)
	linesTable(m, st)
	totalsRow(m, st)
	footer(m, st)

	doc, err := m.Generate()
	if err != nil {
		return fmt.Errorf("maroto generate: %w", err)
	}
	if _, err := w.Write(doc.GetBytes()); err != nil {
		return fmt.Errorf("write pdf: %w", err)
	}
	return nil
}

func header(m core.Maroto, st *pdfdoc.Statement) {
	m.AddRow(8,
		col.New(8).Add(
			text.New("INVOICE", props.Text{Size: 22, Style: fontstyle.Bold}),
		),
		col.New(4).Add(
			text.New(st.Header.InvoiceNo, props.Text{Size: 12, Style: fontstyle.Bold, Align: align.Right}),
		),
	)
	m.AddRow(2)
}

func addressBlock(m core.Maroto, st *pdfdoc.Statement) {
	// Issuer (left) | Bill To (right)
	m.AddRow(36,
		col.New(6).Add(
			text.New("FROM", props.Text{Size: 8, Style: fontstyle.Bold, Color: gray()}),
			text.New(st.Issuer.Name, props.Text{Top: 4, Size: 11, Style: fontstyle.Bold}),
			text.New(st.Issuer.Address, props.Text{Top: 10, Size: 9}),
			text.New(st.Issuer.Email, props.Text{Top: 16, Size: 9}),
		),
		col.New(6).Add(
			text.New("BILL TO", props.Text{Size: 8, Style: fontstyle.Bold, Color: gray()}),
			text.New(st.Counterparty.Name, props.Text{Top: 4, Size: 11, Style: fontstyle.Bold}),
			text.New(st.Counterparty.Address, props.Text{Top: 10, Size: 9}),
			text.New(st.Counterparty.Email, props.Text{Top: 16, Size: 9}),
		),
	)
}

func invoiceMeta(m core.Maroto, st *pdfdoc.Statement) {
	m.AddRow(6,
		col.New(3).Add(text.New("Issue date:", props.Text{Size: 9, Style: fontstyle.Bold})),
		col.New(3).Add(text.New(st.Header.IssueDate, props.Text{Size: 9})),
		col.New(3).Add(text.New("Due date:", props.Text{Size: 9, Style: fontstyle.Bold})),
		col.New(3).Add(text.New(st.Header.DueDate, props.Text{Size: 9})),
	)
	m.AddRow(6,
		col.New(3).Add(text.New("Terms:", props.Text{Size: 9, Style: fontstyle.Bold})),
		col.New(3).Add(text.New(st.Header.PaymentTerms, props.Text{Size: 9})),
		col.New(3).Add(text.New("Status:", props.Text{Size: 9, Style: fontstyle.Bold})),
		col.New(3).Add(text.New(st.Header.Status, props.Text{Size: 9})),
	)
	m.AddRow(4)
}

func linesTable(m core.Maroto, st *pdfdoc.Statement) {
	// Header
	headerStyle := props.Text{Size: 9, Style: fontstyle.Bold, Color: white(), Align: align.Left}
	m.AddRow(7,
		col.New(2).Add(text.New("Date", headerStyle)),
		col.New(2).Add(text.New("Ref", headerStyle)),
		col.New(4).Add(text.New("Description", headerStyle)),
		col.New(2).Add(text.New("Rate", headerStyle)),
		col.New(1).Add(text.New("Hrs", props.Text{Size: 9, Style: fontstyle.Bold, Color: white(), Align: align.Right})),
		col.New(1).Add(text.New("Amount", props.Text{Size: 9, Style: fontstyle.Bold, Color: white(), Align: align.Right})),
	).WithStyle(&props.Cell{BackgroundColor: dark()})

	body := props.Text{Size: 9}
	right := props.Text{Size: 9, Align: align.Right}
	for _, ln := range st.Lines {
		m.AddRow(6,
			col.New(2).Add(text.New(ln.Date, body)),
			col.New(2).Add(text.New(ln.Ref, body)),
			col.New(4).Add(text.New(ln.Desc, body)),
			col.New(2).Add(text.New(ln.RateLabel, body)),
			col.New(1).Add(text.New(ln.HoursLabel, right)),
			col.New(1).Add(text.New(ln.AmountLabel, right)),
		)
	}
}

func totalsRow(m core.Maroto, st *pdfdoc.Statement) {
	m.AddRow(2)
	m.AddRow(7,
		col.New(8),
		col.New(2).Add(text.New("Total", props.Text{Size: 11, Style: fontstyle.Bold, Align: align.Right})),
		col.New(1).Add(text.New(fmt.Sprintf("%.1f", st.TotalHours), props.Text{Size: 11, Style: fontstyle.Bold, Align: align.Right})),
		col.New(1).Add(text.New(money.FormatUSD(st.TotalCents), props.Text{Size: 11, Style: fontstyle.Bold, Align: align.Right})),
	)
}

func footer(m core.Maroto, st *pdfdoc.Statement) {
	m.AddRow(6)
	m.AddRow(28,
		col.New(7).Add(
			text.New("Payment details", props.Text{Size: 8, Style: fontstyle.Bold, Color: gray()}),
			text.New(st.Bank.Name, props.Text{Top: 4, Size: 9, Style: fontstyle.Bold}),
			text.New("Account: "+st.Bank.AccountNumber, props.Text{Top: 9, Size: 9}),
			text.New("SWIFT: "+st.Bank.SWIFT, props.Text{Top: 14, Size: 9}),
			text.New(st.Bank.Address, props.Text{Top: 19, Size: 9}),
		),
		col.New(5).Add(
			text.New("Notes", props.Text{Size: 8, Style: fontstyle.Bold, Color: gray()}),
			text.New(st.Notes, props.Text{Top: 4, Size: 9}),
		),
	)
	m.AddRow(2)
	tagline := fmt.Sprintf("Generated %s", st.Footer.GeneratedAt)
	if st.Footer.GitShortSHA != "" {
		tagline += " · " + st.Footer.GitShortSHA
	}
	m.AddRow(4,
		col.New(12).Add(
			text.New(tagline, props.Text{Size: 7, Color: gray(), Align: align.Center}),
		),
	)
}

// Color helpers — kept inline (so we don't pull a "theme" abstraction we don't need).
func gray() *props.Color  { return &props.Color{Red: 90, Green: 90, Blue: 90} }
func white() *props.Color { return &props.Color{Red: 255, Green: 255, Blue: 255} }
func dark() *props.Color  { return &props.Color{Red: 40, Green: 40, Blue: 40} }
