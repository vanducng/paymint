package pdfdoc_test

import (
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vanducng/paymint/internal/core/config"
	"github.com/vanducng/paymint/internal/core/ledger"
	"github.com/vanducng/paymint/internal/core/model"
	"github.com/vanducng/paymint/internal/core/pdfdoc"
)

// fixtureABS returns a ledger seeded with the canonical reference invoice.
func fixtureABS(t *testing.T) (*ledger.Ledger, *config.Config) {
	t.Helper()
	l := ledger.New()
	require.NoError(t, l.AddCompany(&model.Company{
		ID: "abs", Slug: "abs", Name: "Adventure Bound Studio", Currency: "USD",
		Address: "123 Studio Lane",
	}))
	require.NoError(t, l.AddContract(&model.Contract{
		ID: "abs-c", CompanyID: "abs", Title: "Consulting",
		DefaultRate: 8500,
		Start:       civil.Date{Year: 2026, Month: time.April, Day: 1},
	}))
	require.NoError(t, l.AddInvoice(&model.Invoice{
		ID: "INV-abs-202604", CompanyID: "abs", ContractID: "abs-c",
		IssueDate:  civil.Date{Year: 2026, Month: time.April, Day: 2},
		DueDate:    civil.Date{Year: 2026, Month: time.April, Day: 17},
		TotalCents: 38250, Status: model.StatusIssued,
		Lines: []*model.InvoiceLine{
			{
				ID: "inv-abs-202604-l01", InvoiceID: "INV-abs-202604",
				Date:        civil.Date{Year: 2026, Month: time.April, Day: 2},
				Description: "Explore the API",
				Hours:       4.0, AmountCents: 34000,
			},
			{
				ID: "inv-abs-202604-l02", InvoiceID: "INV-abs-202604",
				Date:        civil.Date{Year: 2026, Month: time.April, Day: 4},
				Description: "Standup",
				Hours:       0.5, AmountCents: 4250,
			},
		},
	}))
	cfg := &config.Config{
		Issuer: config.Issuer{
			Name: "Duc Nguyen", Address: "Hanoi", Email: "me@vanducng.dev",
			Bank: config.Bank{
				Name: "Vietcombank", AccountNumber: "0011001234567", SWIFT: "BFTVVNVX",
			},
		},
	}
	return l, cfg
}

func TestBuildInvoice_Shape(t *testing.T) {
	l, cfg := fixtureABS(t)
	st, err := pdfdoc.BuildInvoice(l, "INV-abs-202604", cfg, pdfdoc.Footer{
		GeneratedAt: "2026-05-02T22:00:00Z",
	})
	require.NoError(t, err)

	assert.Equal(t, "INV-abs-202604", st.Header.InvoiceNo)
	assert.Equal(t, "2026-04-02", st.Header.IssueDate)
	assert.Equal(t, "2026-04-17", st.Header.DueDate)
	assert.Equal(t, "Net 15", st.Header.PaymentTerms)
	assert.Equal(t, "issued", st.Header.Status)

	assert.Equal(t, "Adventure Bound Studio", st.Counterparty.Name)
	assert.Equal(t, "Duc Nguyen", st.Issuer.Name)
	assert.Equal(t, "Vietcombank", st.Bank.Name)

	require.Len(t, st.Lines, 2)
	assert.Equal(t, "Explore the API", st.Lines[0].Desc)
	assert.Equal(t, "$85.00/hr", st.Lines[0].RateLabel)
	assert.Equal(t, "4.0", st.Lines[0].HoursLabel)
	assert.Equal(t, "$340.00", st.Lines[0].AmountLabel)

	assert.Equal(t, int64(38250), st.TotalCents)
	assert.InDelta(t, float32(4.5), st.TotalHours, 0.001)
}

func TestBuildInvoice_RejectsUnknown(t *testing.T) {
	l, cfg := fixtureABS(t)
	_, err := pdfdoc.BuildInvoice(l, "INV-nope-202604", cfg, pdfdoc.Footer{})
	assert.ErrorContains(t, err, "not found")
}

func TestPaymentTerms_OnReceipt(t *testing.T) {
	// when issue == due, PDF renders "On receipt".
	l := ledger.New()
	require.NoError(t, l.AddCompany(&model.Company{ID: "x", Slug: "x", Name: "X", Currency: "USD"}))
	require.NoError(t, l.AddInvoice(&model.Invoice{
		ID: "INV-x-202604", CompanyID: "x",
		IssueDate:  civil.Date{Year: 2026, Month: time.April, Day: 2},
		DueDate:    civil.Date{Year: 2026, Month: time.April, Day: 2},
		TotalCents: 0, Status: model.StatusIssued,
	}))
	st, err := pdfdoc.BuildInvoice(l, "INV-x-202604",
		&config.Config{Issuer: config.Issuer{Name: "x"}}, pdfdoc.Footer{})
	require.NoError(t, err)
	assert.Equal(t, "On receipt", st.Header.PaymentTerms)
}
