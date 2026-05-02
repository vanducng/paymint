package model

import (
	"errors"
	"fmt"

	"cloud.google.com/go/civil"
)

// Payment is one transfer received against an invoice.
type Payment struct {
	ID          string     `yaml:"id"`
	InvoiceID   string     `yaml:"invoice_id"`
	Date        civil.Date `yaml:"date"`
	AmountCents int64      `yaml:"amount_cents"`
	Method      string     `yaml:"method,omitempty"`    // wire, check, etc.
	Reference   string     `yaml:"reference,omitempty"` // bank ref / wire ID
	Notes       string     `yaml:"notes,omitempty"`
}

// Validate asserts the payment's invariants. invoiceTotal is the parent
// invoice's total; pass 0 to skip overpayment checks (left to the ledger).
func (p *Payment) Validate() error {
	var errs []error
	if err := ValidateID(p.ID); err != nil {
		errs = append(errs, fmt.Errorf("payment.id: %w", err))
	}
	if p.InvoiceID == "" {
		errs = append(errs, errors.New("payment.invoice_id: required"))
	}
	if !p.Date.IsValid() {
		errs = append(errs, errors.New("payment.date: invalid"))
	}
	if p.AmountCents <= 0 {
		errs = append(errs, fmt.Errorf("payment.amount_cents: must be > 0, got %d", p.AmountCents))
	}
	return errors.Join(errs...)
}
