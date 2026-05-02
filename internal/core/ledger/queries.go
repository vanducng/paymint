package ledger

import (
	"sort"

	"cloud.google.com/go/civil"

	"github.com/vanducng/paymint/internal/core/model"
	"github.com/vanducng/paymint/internal/core/period"
)

// InvoicesIn returns invoices issued in the given period, optionally filtered
// by company. Pass empty string for companyID to include all companies.
// Result is sorted by issue date then invoice ID for determinism.
func (l *Ledger) InvoicesIn(p period.YearMonth, companyID string) []*model.Invoice {
	var out []*model.Invoice
	for _, inv := range l.Invoices {
		if companyID != "" && inv.CompanyID != companyID {
			continue
		}
		if period.FromDate(inv.IssueDate) != p {
			continue
		}
		out = append(out, inv)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IssueDate != out[j].IssueDate {
			return out[i].IssueDate.Before(out[j].IssueDate)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// PaymentsFor returns payments applied to the given invoice ID, sorted by
// date then payment ID.
func (l *Ledger) PaymentsFor(invoiceID string) []*model.Payment {
	var out []*model.Payment
	for _, p := range l.Payments {
		if p.InvoiceID == invoiceID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date.Before(out[j].Date)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// PaidCents returns the cumulative paid amount on an invoice through asOf
// (inclusive). Pass civil.Date{} to include every payment.
func (l *Ledger) PaidCents(invoiceID string, asOf civil.Date) int64 {
	var sum int64
	for _, p := range l.Payments {
		if p.InvoiceID != invoiceID {
			continue
		}
		if (asOf != civil.Date{}) && p.Date.After(asOf) {
			continue
		}
		sum += p.AmountCents
	}
	return sum
}

// Outstanding returns the total unpaid balance for a company as of asOf
// (inclusive). Revoked invoices are excluded. Result is in cents.
func (l *Ledger) Outstanding(companyID string, asOf civil.Date) int64 {
	var owed int64
	for _, inv := range l.Invoices {
		if inv.CompanyID != companyID {
			continue
		}
		if inv.Status == model.StatusRevoked {
			continue
		}
		if (asOf != civil.Date{}) && inv.IssueDate.After(asOf) {
			continue
		}
		owed += inv.TotalCents - l.PaidCents(inv.ID, asOf)
	}
	return owed
}
