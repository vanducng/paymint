package model

import (
	"errors"
	"fmt"

	"cloud.google.com/go/civil"
)

// Contract represents an engagement with a Company. DefaultRate is in cents
// per hour; InvoiceLines may override it per row.
type Contract struct {
	ID          string     `yaml:"id"`
	CompanyID   string     `yaml:"company_id"`
	Title       string     `yaml:"title"`
	DefaultRate int64      `yaml:"default_rate_cents"` // cents per hour
	Start       civil.Date `yaml:"start"`
	End         civil.Date `yaml:"end,omitempty"` // zero value = open
	Cadence     string     `yaml:"cadence,omitempty"`
	DocURL      string     `yaml:"doc_url,omitempty"`
	Notes       string     `yaml:"notes,omitempty"`
}

// Active reports whether the contract is in force on the given date.
func (c *Contract) Active(asOf civil.Date) bool {
	if asOf.Before(c.Start) {
		return false
	}
	if (c.End == civil.Date{}) {
		return true
	}
	return !c.End.Before(asOf)
}

// Validate asserts the contract's invariants.
func (c *Contract) Validate() error {
	var errs []error
	if err := ValidateID(c.ID); err != nil {
		errs = append(errs, fmt.Errorf("contract.id: %w", err))
	}
	if err := ValidateID(c.CompanyID); err != nil {
		errs = append(errs, fmt.Errorf("contract.company_id: %w", err))
	}
	if c.Title == "" {
		errs = append(errs, errors.New("contract.title: required"))
	}
	if c.DefaultRate < 0 {
		errs = append(errs, fmt.Errorf("contract.default_rate_cents: must be >= 0, got %d", c.DefaultRate))
	}
	if !c.Start.IsValid() {
		errs = append(errs, errors.New("contract.start: invalid date"))
	}
	if (c.End != civil.Date{}) && c.End.Before(c.Start) {
		errs = append(errs, errors.New("contract.end: must be on or after start"))
	}
	return errors.Join(errs...)
}
