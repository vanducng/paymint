package ledger_test

import (
	"math"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vanducng/paymint/internal/core/ledger"
	"github.com/vanducng/paymint/internal/core/model"
)

// fixtureABS returns a ledger seeded with the canonical ABS reference invoice:
// 1 company (abs / Adventure Bound Studio), 1 contract @ $85/hr, 1 invoice
// (INV-abs-202604) with 8 lines totalling 16.5h / $1,402.50.
func fixtureABS(t *testing.T) *ledger.Ledger {
	t.Helper()
	l := ledger.New()

	require.NoError(t, l.AddCompany(&model.Company{
		ID:       "abs",
		Slug:     "abs",
		Name:     "Adventure Bound Studio",
		Currency: "USD",
	}))
	require.NoError(t, l.AddContract(&model.Contract{
		ID:          "abs-consulting",
		CompanyID:   "abs",
		Title:       "Consulting",
		DefaultRate: 8500, // $85.00/hr
		Start:       civil.Date{Year: 2026, Month: time.April, Day: 1},
	}))

	hours := []float32{4.0, 0.5, 3.0, 2.0, 4.0, 1.0, 1.5, 0.5}
	descs := []string{
		"Explore the API",
		"Standup",
		"Schema design",
		"Pair on auth",
		"Implement webhook",
		"Code review",
		"Bug triage",
		"Wrap-up call",
	}
	require.Equal(t, len(hours), len(descs))

	var lines []*model.InvoiceLine
	var total int64
	for i, h := range hours {
		amt := int64(math.Round(8500 * float64(h)))
		total += amt
		lines = append(lines, &model.InvoiceLine{
			ID:          mustLineID(i),
			InvoiceID:   "INV-abs-202604",
			Date:        civil.Date{Year: 2026, Month: time.April, Day: 1 + i},
			Description: descs[i],
			Hours:       h,
			AmountCents: amt,
		})
	}
	assert.Equal(t, int64(140250), total) // $1,402.50

	require.NoError(t, l.AddInvoice(&model.Invoice{
		ID:         "INV-abs-202604",
		CompanyID:  "abs",
		ContractID: "abs-consulting",
		IssueDate:  civil.Date{Year: 2026, Month: time.April, Day: 2},
		DueDate:    civil.Date{Year: 2026, Month: time.April, Day: 17},
		TotalCents: total,
		Status:     model.StatusIssued,
		Lines:      lines,
	}))
	return l
}

// mustLineID produces deterministic IDs for fixture lines.
func mustLineID(i int) string {
	return "abs-line-" + string(rune('a'+i))
}

func TestFixtureABS_TotalsAndQueries(t *testing.T) {
	l := fixtureABS(t)

	inv := l.Invoices["INV-abs-202604"]
	require.NotNil(t, inv)
	assert.Equal(t, int64(140250), inv.TotalCents)
	assert.Len(t, inv.Lines, 8)

	// Outstanding before any payment = full total.
	asOf := civil.Date{Year: 2026, Month: time.April, Day: 30}
	assert.Equal(t, int64(140250), l.Outstanding("abs", asOf))

	// Add a partial payment, then a closing payment.
	require.NoError(t, l.AddPayment(&model.Payment{
		ID:          "pay-001",
		InvoiceID:   "INV-abs-202604",
		Date:        civil.Date{Year: 2026, Month: time.April, Day: 18},
		AmountCents: 100000,
		Method:      "wire",
	}))
	require.NoError(t, l.AddPayment(&model.Payment{
		ID:          "pay-002",
		InvoiceID:   "INV-abs-202604",
		Date:        civil.Date{Year: 2026, Month: time.April, Day: 20},
		AmountCents: 40250,
		Method:      "wire",
	}))
	assert.Equal(t, int64(0), l.Outstanding("abs", asOf))
	assert.Len(t, l.PaymentsFor("INV-abs-202604"), 2)

	// PaidCents respects the asOf cutoff.
	cutoff := civil.Date{Year: 2026, Month: time.April, Day: 19}
	assert.Equal(t, int64(100000), l.PaidCents("INV-abs-202604", cutoff))
}

func TestAddInvoice_Validation(t *testing.T) {
	l := fixtureABS(t)

	// Duplicate invoice id rejected.
	dup := *l.Invoices["INV-abs-202604"]
	dup.Lines = nil
	dup.TotalCents = 0
	err := l.AddInvoice(&dup)
	assert.ErrorContains(t, err, "already exists")

	// Slug mismatch on ID rejected.
	bad := &model.Invoice{
		ID:         "INV-other-202604",
		CompanyID:  "abs",
		IssueDate:  civil.Date{Year: 2026, Month: time.April, Day: 2},
		DueDate:    civil.Date{Year: 2026, Month: time.April, Day: 15},
		TotalCents: 0,
		Status:     model.StatusIssued,
	}
	assert.ErrorContains(t, l.AddInvoice(bad), "does not match company slug")

	// Embedded YYYYMM mismatch with issue date rejected.
	mismatch := &model.Invoice{
		ID:         "INV-abs-202607",
		CompanyID:  "abs",
		IssueDate:  civil.Date{Year: 2026, Month: time.April, Day: 2},
		DueDate:    civil.Date{Year: 2026, Month: time.April, Day: 15},
		TotalCents: 0,
		Status:     model.StatusIssued,
	}
	assert.ErrorContains(t, l.AddInvoice(mismatch), "does not match issue date")
}

func TestAddPayment_Validation(t *testing.T) {
	l := fixtureABS(t)

	// Unknown invoice rejected.
	err := l.AddPayment(&model.Payment{
		ID:          "pay-bad",
		InvoiceID:   "INV-nope-202604",
		Date:        civil.Date{Year: 2026, Month: time.April, Day: 18},
		AmountCents: 100,
	})
	assert.ErrorContains(t, err, "unknown invoice")

	// Non-positive amount rejected.
	err = l.AddPayment(&model.Payment{
		ID:          "pay-zero",
		InvoiceID:   "INV-abs-202604",
		Date:        civil.Date{Year: 2026, Month: time.April, Day: 18},
		AmountCents: 0,
	})
	assert.ErrorContains(t, err, "must be > 0")
}

func TestDirty_Tracking(t *testing.T) {
	l := fixtureABS(t)
	d := l.Dirty()
	assert.True(t, d.Companies)
	assert.True(t, d.Contracts)
	require.Len(t, d.InvoiceMonths, 1)
	assert.Equal(t, time.April, d.InvoiceMonths[0].Month)
	assert.Equal(t, 2026, d.InvoiceMonths[0].Year)
	assert.Empty(t, d.PaymentMonths)

	l.MarkClean()
	assert.Empty(t, l.Dirty().InvoiceMonths)
	assert.False(t, l.Dirty().Companies)
}

func TestMarkInvoiceStatus(t *testing.T) {
	l := fixtureABS(t)
	l.MarkClean()

	require.NoError(t, l.MarkInvoiceStatus("INV-abs-202604", model.StatusPaid))
	assert.Equal(t, model.StatusPaid, l.Invoices["INV-abs-202604"].Status)

	d := l.Dirty()
	require.Len(t, d.InvoiceMonths, 1)
}
