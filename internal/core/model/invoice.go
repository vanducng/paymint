package model

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"cloud.google.com/go/civil"
)

// InvoiceStatus enumerates the lifecycle states of an invoice.
type InvoiceStatus string

// InvoiceStatus values. Status transitions are not enforced at the model
// layer — the CLI / sync code owns the state machine.
const (
	StatusDraft   InvoiceStatus = "draft"
	StatusIssued  InvoiceStatus = "issued"
	StatusPaid    InvoiceStatus = "paid"
	StatusOverdue InvoiceStatus = "overdue"
	StatusRevoked InvoiceStatus = "revoked"
)

// Valid reports whether s is one of the known InvoiceStatus values.
func (s InvoiceStatus) Valid() bool {
	switch s {
	case StatusDraft, StatusIssued, StatusPaid, StatusOverdue, StatusRevoked:
		return true
	}
	return false
}

// Invoice is one billable document. Total is denormalized (== Σ Lines.Amount)
// to allow rendering without re-walking every line.
type Invoice struct {
	ID         string         `yaml:"id"` // INV-<slug>-<YYYYMM>
	CompanyID  string         `yaml:"company_id"`
	ContractID string         `yaml:"contract_id,omitempty"`
	IssueDate  civil.Date     `yaml:"issue_date"`
	DueDate    civil.Date     `yaml:"due_date"`
	TotalCents int64          `yaml:"total_cents"`
	Status     InvoiceStatus  `yaml:"status"`
	Notes      string         `yaml:"notes,omitempty"`
	Lines      []*InvoiceLine `yaml:"lines,omitempty"`
}

// InvoiceLine is a single hourly entry on an invoice.
type InvoiceLine struct {
	ID          string     `yaml:"id"`
	InvoiceID   string     `yaml:"invoice_id"`
	Date        civil.Date `yaml:"date"`
	Description string     `yaml:"description"`
	Ref         string     `yaml:"ref,omitempty"` // jira / meeting reference
	RateCents   int64      `yaml:"rate_cents"`    // 0 = use contract default
	Hours       float32    `yaml:"hours"`
	AmountCents int64      `yaml:"amount_cents"` // == round(Hours * effectiveRate)
}

// EffectiveRate returns the per-hour cents rate applied to this line, falling
// back to fallback (typically Contract.DefaultRate) when the line's own rate
// is zero.
func (l *InvoiceLine) EffectiveRate(fallback int64) int64 {
	if l.RateCents > 0 {
		return l.RateCents
	}
	return fallback
}

// Validate asserts the line's invariants. fallbackRate is the contract default
// (0 if unknown to the caller); the validator allows zero amount mismatch
// when fallbackRate is also 0 (caller validates separately).
func (l *InvoiceLine) Validate(fallbackRate int64) error {
	var errs []error
	if err := ValidateID(l.ID); err != nil {
		errs = append(errs, fmt.Errorf("line.id: %w", err))
	}
	if l.Description == "" {
		errs = append(errs, errors.New("line.description: required"))
	}
	if !l.Date.IsValid() {
		errs = append(errs, errors.New("line.date: invalid"))
	}
	if l.Hours <= 0 {
		errs = append(errs, fmt.Errorf("line.hours: must be > 0, got %v", l.Hours))
	}
	if l.RateCents < 0 {
		errs = append(errs, fmt.Errorf("line.rate_cents: must be >= 0, got %d", l.RateCents))
	}
	if l.AmountCents < 0 {
		errs = append(errs, fmt.Errorf("line.amount_cents: must be >= 0, got %d", l.AmountCents))
	}
	if rate := l.EffectiveRate(fallbackRate); rate > 0 && l.Hours > 0 {
		expected := int64(math.Round(float64(rate) * float64(l.Hours)))
		// allow ±1 cent rounding tolerance
		if diff := expected - l.AmountCents; diff < -1 || diff > 1 {
			errs = append(errs, fmt.Errorf(
				"line.amount_cents: expected %d (rate=%d cents/hr × %v hrs), got %d",
				expected, rate, l.Hours, l.AmountCents))
		}
	}
	return errors.Join(errs...)
}

// Validate asserts the invoice's invariants. companySlug is used to verify
// the embedded slug in the invoice ID matches the referenced company.
func (i *Invoice) Validate(companySlug string, contractRate int64) error {
	var errs []error
	slug, year, month, err := ValidateInvoiceID(i.ID)
	if err != nil {
		errs = append(errs, fmt.Errorf("invoice.id: %w", err))
	} else if companySlug != "" && !strings.EqualFold(slug, companySlug) {
		errs = append(errs, fmt.Errorf(
			"invoice.id: slug %q does not match company slug %q", slug, companySlug))
	} else if i.IssueDate.IsValid() {
		if year != i.IssueDate.Year || int(i.IssueDate.Month) != month {
			errs = append(errs, fmt.Errorf(
				"invoice.id: embedded YYYYMM %04d%02d does not match issue date %s",
				year, month, i.IssueDate))
		}
	}
	if err := ValidateID(i.CompanyID); err != nil {
		errs = append(errs, fmt.Errorf("invoice.company_id: %w", err))
	}
	if i.ContractID != "" {
		if err := ValidateID(i.ContractID); err != nil {
			errs = append(errs, fmt.Errorf("invoice.contract_id: %w", err))
		}
	}
	if !i.IssueDate.IsValid() {
		errs = append(errs, errors.New("invoice.issue_date: invalid"))
	}
	if !i.DueDate.IsValid() {
		errs = append(errs, errors.New("invoice.due_date: invalid"))
	}
	if i.IssueDate.IsValid() && i.DueDate.IsValid() && i.DueDate.Before(i.IssueDate) {
		errs = append(errs, errors.New("invoice.due_date: must be on or after issue_date"))
	}
	if i.TotalCents < 0 {
		errs = append(errs, fmt.Errorf("invoice.total_cents: must be >= 0, got %d", i.TotalCents))
	}
	if !i.Status.Valid() {
		errs = append(errs, fmt.Errorf("invoice.status: unknown %q", i.Status))
	}

	var lineSum int64
	for idx, l := range i.Lines {
		if l.InvoiceID != "" && l.InvoiceID != i.ID {
			errs = append(errs, fmt.Errorf("invoice.lines[%d].invoice_id: %q does not match parent %q", idx, l.InvoiceID, i.ID))
		}
		if err := l.Validate(contractRate); err != nil {
			errs = append(errs, fmt.Errorf("invoice.lines[%d]: %w", idx, err))
		}
		lineSum += l.AmountCents
	}
	if len(i.Lines) > 0 && lineSum != i.TotalCents {
		errs = append(errs, fmt.Errorf("invoice.total_cents: %d does not equal Σ lines (%d)", i.TotalCents, lineSum))
	}
	return errors.Join(errs...)
}
