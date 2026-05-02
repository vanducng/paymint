package pdf_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vanducng/paymint/internal/core/config"
	"github.com/vanducng/paymint/internal/core/pdfdoc"
	"github.com/vanducng/paymint/internal/pdf"
)

func TestRender_ProducesValidPDF(t *testing.T) {
	st := &pdfdoc.Statement{
		Issuer: config.Issuer{Name: "Duc Nguyen", Address: "Hanoi", Email: "me@vanducng.dev"},
		Bank:   config.Bank{Name: "Vietcombank", AccountNumber: "0011", SWIFT: "BFTV"},
		Counterparty: pdfdoc.Counterparty{
			ID: "abs", Name: "Adventure Bound Studio",
			Address: "123 Studio Lane", Email: "biz@abs.test",
		},
		Header: pdfdoc.InvoiceHeader{
			InvoiceNo: "INV-abs-202604",
			IssueDate: "2026-04-02", DueDate: "2026-04-17",
			PaymentTerms: "Net 15", Status: "issued",
		},
		Lines: []pdfdoc.Line{
			{Date: "2026-04-02", Desc: "Explore the API",
				RateLabel: "$85.00/hr", HoursLabel: "4.0", AmountLabel: "$340.00"},
		},
		TotalCents: 34000, TotalHours: 4.0,
		Footer: pdfdoc.Footer{GeneratedAt: "2026-05-02T22:00:00Z"},
	}

	var buf bytes.Buffer
	require.NoError(t, pdf.Render(st, &buf))

	out := buf.Bytes()
	assert.Greater(t, len(out), 1024, "expected non-trivial PDF size")
	assert.True(t, bytes.HasPrefix(out, []byte("%PDF-")), "expected PDF magic bytes")
}
