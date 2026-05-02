// Package model defines the canonical paymint domain entities. All structs
// are pure data — no I/O, no clocks, no logging.
package model

import (
	"errors"
	"fmt"
)

// Company is a billable counterparty.
type Company struct {
	ID       string `yaml:"id"`
	Slug     string `yaml:"slug"`
	Name     string `yaml:"name"`
	Currency string `yaml:"currency"` // v0.1: must be "USD"
	TaxID    string `yaml:"tax_id,omitempty"`
	Address  string `yaml:"address,omitempty"`
	Email    string `yaml:"email,omitempty"`
}

// Validate asserts the company is internally consistent.
func (c *Company) Validate() error {
	var errs []error
	if err := ValidateID(c.ID); err != nil {
		errs = append(errs, fmt.Errorf("company.id: %w", err))
	}
	if err := ValidateID(c.Slug); err != nil {
		errs = append(errs, fmt.Errorf("company.slug: %w", err))
	}
	if c.Name == "" {
		errs = append(errs, errors.New("company.name: required"))
	}
	if c.Currency != "USD" {
		errs = append(errs, fmt.Errorf("company.currency: only USD supported in v0.1, got %q", c.Currency))
	}
	return errors.Join(errs...)
}
