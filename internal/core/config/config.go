// Package config defines the on-disk paymint configuration. Loaded from
// .paymint/config.yaml; pure data with a Validate method (no I/O).
package config

import (
	"errors"
	"fmt"
)

// Config holds the per-install paymint configuration.
type Config struct {
	SheetID  string `yaml:"sheet_id,omitempty"`
	ClientID string `yaml:"oauth_client_id,omitempty"`
	Issuer   Issuer `yaml:"issuer"`
}

// Issuer is the single billing entity that owns this paymint install
// — your name and bank details, rendered onto every PDF.
type Issuer struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address,omitempty"`
	Email   string `yaml:"email,omitempty"`
	Bank    Bank   `yaml:"bank"`
}

// Bank is the issuer's payment-receiving account.
type Bank struct {
	Name          string `yaml:"name"`
	AccountNumber string `yaml:"account_number"`
	SWIFT         string `yaml:"swift,omitempty"`
	Address       string `yaml:"address,omitempty"`
}

// Validate asserts the config has the minimum required fields populated.
// The CLI tolerates a partial config in `paymint init`, but operations that
// produce PDFs or Sheets writes call Validate first.
func (c *Config) Validate() error {
	var errs []error
	if c.Issuer.Name == "" {
		errs = append(errs, errors.New("issuer.name: required"))
	}
	if c.Issuer.Bank.Name == "" {
		errs = append(errs, errors.New("issuer.bank.name: required"))
	}
	if c.Issuer.Bank.AccountNumber == "" {
		errs = append(errs, errors.New("issuer.bank.account_number: required"))
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return nil
}
