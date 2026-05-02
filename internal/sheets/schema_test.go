package sheets

import (
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vanducng/paymint/internal/core/model"
)

func TestCompanyRoundTrip(t *testing.T) {
	in := &model.Company{
		ID: "abs", Slug: "abs", Name: "ABS", Currency: "USD",
		TaxID: "VN-1234", Email: "biz@abs.test",
	}
	row := CompanyToRow(in, "op-1")
	out, opID, err := RowToCompany(row)
	require.NoError(t, err)
	assert.Equal(t, "op-1", opID)
	assert.Equal(t, in.ID, out.ID)
	assert.Equal(t, in.Slug, out.Slug)
	assert.Equal(t, in.TaxID, out.TaxID)
}

func TestContractRoundTrip(t *testing.T) {
	in := &model.Contract{
		ID: "abs-c", CompanyID: "abs", Title: "Consulting",
		DefaultRate: 8500,
		Start:       civil.Date{Year: 2026, Month: time.April, Day: 1},
		End:         civil.Date{Year: 2027, Month: time.April, Day: 1},
	}
	row := ContractToRow(in, "op-2")
	out, opID, err := RowToContract(row)
	require.NoError(t, err)
	assert.Equal(t, "op-2", opID)
	assert.Equal(t, in.DefaultRate, out.DefaultRate)
	assert.Equal(t, in.Start, out.Start)
	assert.Equal(t, in.End, out.End)
}

func TestInvoiceRoundTrip(t *testing.T) {
	in := &model.Invoice{
		ID: "INV-abs-202604", CompanyID: "abs", ContractID: "abs-c",
		IssueDate:  civil.Date{Year: 2026, Month: time.April, Day: 2},
		DueDate:    civil.Date{Year: 2026, Month: time.April, Day: 17},
		TotalCents: 140250,
		Status:     model.StatusIssued,
		Notes:      "consulting",
	}
	row := InvoiceToRow(in, "op-3")
	out, opID, err := RowToInvoice(row)
	require.NoError(t, err)
	assert.Equal(t, "op-3", opID)
	assert.Equal(t, in.TotalCents, out.TotalCents)
	assert.Equal(t, in.Status, out.Status)
}

func TestLineRoundTrip(t *testing.T) {
	in := &model.InvoiceLine{
		ID: "inv-abs-202604-l01", InvoiceID: "INV-abs-202604",
		Date:        civil.Date{Year: 2026, Month: time.April, Day: 2},
		Description: "Explore the API",
		Hours:       4.0, RateCents: 0, AmountCents: 34000,
	}
	row := LineToRow(in, "op-4")
	out, opID, err := RowToLine(row)
	require.NoError(t, err)
	assert.Equal(t, "op-4", opID)
	assert.Equal(t, in.AmountCents, out.AmountCents)
	assert.InDelta(t, in.Hours, out.Hours, 0.001)
}

func TestPaymentRoundTrip(t *testing.T) {
	in := &model.Payment{
		ID: "p1", InvoiceID: "INV-abs-202604",
		Date:        civil.Date{Year: 2026, Month: time.April, Day: 20},
		AmountCents: 140250, Method: "wire", Reference: "BANK-XYZ",
	}
	row := PaymentToRow(in, "op-5")
	out, opID, err := RowToPayment(row)
	require.NoError(t, err)
	assert.Equal(t, "op-5", opID)
	assert.Equal(t, in.AmountCents, out.AmountCents)
}
