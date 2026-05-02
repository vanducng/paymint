package pdfdoc

import (
	"fmt"
	"time"

	"cloud.google.com/go/civil"

	"github.com/vanducng/paymint/internal/core/config"
	"github.com/vanducng/paymint/internal/core/ledger"
	"github.com/vanducng/paymint/internal/core/money"
)

// BuildInvoice composes a Statement from a ledger entry and the issuer's
// config. footer.GitShortSHA is optional and may be left empty.
func BuildInvoice(l *ledger.Ledger, invoiceID string, cfg *config.Config, footer Footer) (*Statement, error) {
	inv, ok := l.Invoices[invoiceID]
	if !ok {
		return nil, fmt.Errorf("invoice %q not found", invoiceID)
	}
	co, ok := l.Companies[inv.CompanyID]
	if !ok {
		return nil, fmt.Errorf("invoice %q references unknown company %q", invoiceID, inv.CompanyID)
	}

	terms := paymentTerms(inv.IssueDate, inv.DueDate)

	st := &Statement{
		Issuer: cfg.Issuer,
		Bank:   cfg.Issuer.Bank,
		Counterparty: Counterparty{
			ID: co.ID, Name: co.Name, Address: co.Address,
			Email: co.Email, TaxID: co.TaxID,
		},
		Header: InvoiceHeader{
			InvoiceNo:    inv.ID,
			IssueDate:    inv.IssueDate.String(),
			DueDate:      inv.DueDate.String(),
			PaymentTerms: terms,
			Status:       string(inv.Status),
		},
		TotalCents: inv.TotalCents,
		Notes:      inv.Notes,
		Footer:     footer,
	}

	// Resolve the contract default rate once for line-level fallback display.
	var contractRate int64
	if inv.ContractID != "" {
		if c, ok := l.Contracts[inv.ContractID]; ok {
			contractRate = c.DefaultRate
		}
	}

	for _, ln := range inv.Lines {
		effective := ln.RateCents
		if effective == 0 {
			effective = contractRate
		}
		st.Lines = append(st.Lines, Line{
			Date:        ln.Date.String(),
			Ref:         ln.Ref,
			Desc:        ln.Description,
			RateLabel:   money.FormatUSD(effective) + "/hr",
			HoursLabel:  fmt.Sprintf("%.1f", ln.Hours),
			AmountLabel: money.FormatUSD(ln.AmountCents),
		})
		st.TotalHours += ln.Hours
	}
	return st, nil
}

// paymentTerms returns "Net <N>" for the day-count between issue and due.
// Returns "On receipt" if due == issue. civil.Date is timezone-free, so we
// resolve to a fixed UTC midnight for arithmetic.
func paymentTerms(issue, due civil.Date) string {
	if due == issue || due.Before(issue) {
		return "On receipt"
	}
	a := time.Date(issue.Year, issue.Month, issue.Day, 0, 0, 0, 0, time.UTC)
	b := time.Date(due.Year, due.Month, due.Day, 0, 0, 0, 0, time.UTC)
	days := int(b.Sub(a).Hours() / 24)
	return fmt.Sprintf("Net %d", days)
}
