// Package sheets contains the Sheets v4 client wrapper, the 5-tab schema
// (column order pinned per tab), and pull-side sanitization.
package sheets

import (
	"fmt"
	"strconv"

	"cloud.google.com/go/civil"

	"github.com/vanducng/paymint/internal/core/model"
)

// Tab names. Order here determines pull/push iteration order — sync requires
// Companies/Contracts before Invoices and Payments so refs resolve.
const (
	TabCompanies    = "Companies"
	TabContracts    = "Contracts"
	TabInvoices     = "Invoices"
	TabInvoiceLines = "InvoiceLines"
	TabPayments     = "Payments"
)

// PullOrder is the canonical order used by sync.Pull.
var PullOrder = []string{TabCompanies, TabContracts, TabInvoices, TabInvoiceLines, TabPayments}

// Headers return the pinned column header row for each tab. The trailing
// `op_id` column carries the pending-op UUID for idempotent push (F4).
var Headers = map[string][]string{
	TabCompanies:    {"id", "slug", "name", "currency", "tax_id", "address", "email", "op_id"},
	TabContracts:    {"id", "company_id", "title", "default_rate_cents", "start", "end", "cadence", "doc_url", "notes", "op_id"},
	TabInvoices:     {"id", "company_id", "contract_id", "issue_date", "due_date", "total_cents", "status", "notes", "op_id"},
	TabInvoiceLines: {"id", "invoice_id", "date", "description", "ref", "rate_cents", "hours", "amount_cents", "op_id"},
	TabPayments:     {"id", "invoice_id", "date", "amount_cents", "method", "reference", "notes", "op_id"},
}

// CompanyToRow / RowToCompany — symmetric encoders. opID may be empty.
func CompanyToRow(c *model.Company, opID string) []any {
	return []any{c.ID, c.Slug, c.Name, c.Currency, c.TaxID, c.Address, c.Email, opID}
}

// RowToCompany decodes a sanitized row.
func RowToCompany(row []any) (*model.Company, string, error) {
	if len(row) < 4 {
		return nil, "", fmt.Errorf("companies row: %d cols, need >= 4", len(row))
	}
	return &model.Company{
		ID:       FieldString(row, 0),
		Slug:     FieldString(row, 1),
		Name:     FieldString(row, 2),
		Currency: FieldString(row, 3),
		TaxID:    FieldString(row, 4),
		Address:  FieldString(row, 5),
		Email:    FieldString(row, 6),
	}, FieldString(row, 7), nil
}

// ContractToRow / RowToContract.
func ContractToRow(c *model.Contract, opID string) []any {
	endStr := ""
	if (c.End != civil.Date{}) {
		endStr = c.End.String()
	}
	return []any{c.ID, c.CompanyID, c.Title, c.DefaultRate, c.Start.String(),
		endStr, c.Cadence, c.DocURL, c.Notes, opID}
}

// RowToContract decodes a sanitized row.
func RowToContract(row []any) (*model.Contract, string, error) {
	if len(row) < 5 {
		return nil, "", fmt.Errorf("contracts row: %d cols, need >= 5", len(row))
	}
	rate, err := parseInt(FieldString(row, 3))
	if err != nil {
		return nil, "", fmt.Errorf("contracts.default_rate_cents: %w", err)
	}
	start, err := civil.ParseDate(FieldString(row, 4))
	if err != nil {
		return nil, "", fmt.Errorf("contracts.start: %w", err)
	}
	var end civil.Date
	if s := FieldString(row, 5); s != "" {
		end, err = civil.ParseDate(s)
		if err != nil {
			return nil, "", fmt.Errorf("contracts.end: %w", err)
		}
	}
	return &model.Contract{
		ID:          FieldString(row, 0),
		CompanyID:   FieldString(row, 1),
		Title:       FieldString(row, 2),
		DefaultRate: rate,
		Start:       start,
		End:         end,
		Cadence:     FieldString(row, 6),
		DocURL:      FieldString(row, 7),
		Notes:       FieldString(row, 8),
	}, FieldString(row, 9), nil
}

// InvoiceToRow / RowToInvoice.
func InvoiceToRow(i *model.Invoice, opID string) []any {
	return []any{i.ID, i.CompanyID, i.ContractID, i.IssueDate.String(), i.DueDate.String(),
		i.TotalCents, string(i.Status), i.Notes, opID}
}

// RowToInvoice decodes a sanitized row (lines are loaded separately).
func RowToInvoice(row []any) (*model.Invoice, string, error) {
	if len(row) < 7 {
		return nil, "", fmt.Errorf("invoices row: %d cols, need >= 7", len(row))
	}
	issue, err := civil.ParseDate(FieldString(row, 3))
	if err != nil {
		return nil, "", fmt.Errorf("invoices.issue_date: %w", err)
	}
	due, err := civil.ParseDate(FieldString(row, 4))
	if err != nil {
		return nil, "", fmt.Errorf("invoices.due_date: %w", err)
	}
	total, err := parseInt(FieldString(row, 5))
	if err != nil {
		return nil, "", fmt.Errorf("invoices.total_cents: %w", err)
	}
	return &model.Invoice{
		ID:         FieldString(row, 0),
		CompanyID:  FieldString(row, 1),
		ContractID: FieldString(row, 2),
		IssueDate:  issue,
		DueDate:    due,
		TotalCents: total,
		Status:     model.InvoiceStatus(FieldString(row, 6)),
		Notes:      FieldString(row, 7),
	}, FieldString(row, 8), nil
}

// LineToRow / RowToLine. Hours stored as a stringified float to preserve
// precision exactly; rate / amount are integer cents.
func LineToRow(l *model.InvoiceLine, opID string) []any {
	return []any{l.ID, l.InvoiceID, l.Date.String(), l.Description, l.Ref,
		l.RateCents, fmt.Sprintf("%g", l.Hours), l.AmountCents, opID}
}

// RowToLine decodes a sanitized row.
func RowToLine(row []any) (*model.InvoiceLine, string, error) {
	if len(row) < 8 {
		return nil, "", fmt.Errorf("invoice_lines row: %d cols, need >= 8", len(row))
	}
	date, err := civil.ParseDate(FieldString(row, 2))
	if err != nil {
		return nil, "", fmt.Errorf("invoice_lines.date: %w", err)
	}
	rate, err := parseInt(FieldString(row, 5))
	if err != nil {
		return nil, "", fmt.Errorf("invoice_lines.rate_cents: %w", err)
	}
	hours, err := strconv.ParseFloat(FieldString(row, 6), 32)
	if err != nil {
		return nil, "", fmt.Errorf("invoice_lines.hours: %w", err)
	}
	amount, err := parseInt(FieldString(row, 7))
	if err != nil {
		return nil, "", fmt.Errorf("invoice_lines.amount_cents: %w", err)
	}
	return &model.InvoiceLine{
		ID:          FieldString(row, 0),
		InvoiceID:   FieldString(row, 1),
		Date:        date,
		Description: FieldString(row, 3),
		Ref:         FieldString(row, 4),
		RateCents:   rate,
		Hours:       float32(hours),
		AmountCents: amount,
	}, FieldString(row, 8), nil
}

// PaymentToRow / RowToPayment.
func PaymentToRow(p *model.Payment, opID string) []any {
	return []any{p.ID, p.InvoiceID, p.Date.String(), p.AmountCents, p.Method, p.Reference, p.Notes, opID}
}

// RowToPayment decodes a sanitized row.
func RowToPayment(row []any) (*model.Payment, string, error) {
	if len(row) < 4 {
		return nil, "", fmt.Errorf("payments row: %d cols, need >= 4", len(row))
	}
	date, err := civil.ParseDate(FieldString(row, 2))
	if err != nil {
		return nil, "", fmt.Errorf("payments.date: %w", err)
	}
	amount, err := parseInt(FieldString(row, 3))
	if err != nil {
		return nil, "", fmt.Errorf("payments.amount_cents: %w", err)
	}
	return &model.Payment{
		ID:          FieldString(row, 0),
		InvoiceID:   FieldString(row, 1),
		Date:        date,
		AmountCents: amount,
		Method:      FieldString(row, 4),
		Reference:   FieldString(row, 5),
		Notes:       FieldString(row, 6),
	}, FieldString(row, 7), nil
}

func parseInt(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}
