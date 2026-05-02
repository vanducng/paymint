package yamlstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	yaml "github.com/goccy/go-yaml"

	"github.com/vanducng/paymint/internal/core/ledger"
	"github.com/vanducng/paymint/internal/core/model"
)

// Load reads every shard under p.Root() and returns a fully populated
// in-memory Ledger. Missing files / directories are tolerated (treated as
// empty); duplicate IDs across shards fail fast.
func Load(p *Paths) (*ledger.Ledger, error) {
	l := ledger.New()

	if err := loadCompanies(p, l); err != nil {
		return nil, fmt.Errorf("companies: %w", err)
	}
	if err := loadContracts(p, l); err != nil {
		return nil, fmt.Errorf("contracts: %w", err)
	}
	if err := loadInvoices(p, l); err != nil {
		return nil, fmt.Errorf("invoices: %w", err)
	}
	if err := loadPayments(p, l); err != nil {
		return nil, fmt.Errorf("payments: %w", err)
	}
	if err := l.CrossValidate(); err != nil {
		return nil, fmt.Errorf("cross-validate: %w", err)
	}
	l.MarkClean()
	return l, nil
}

func readYAMLOptional(path string, out any) error {
	// path is constructed from a *Paths instance whose root has been
	// canonicalised; callers never pass user-supplied paths here.
	b, err := os.ReadFile(path) //nolint:gosec // path validated by Paths
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	return yaml.Unmarshal(b, out)
}

func loadCompanies(p *Paths, l *ledger.Ledger) error {
	var companies []*model.Company
	if err := readYAMLOptional(p.Companies(), &companies); err != nil {
		return err
	}
	for _, c := range companies {
		if _, dup := l.Companies[c.ID]; dup {
			return fmt.Errorf("duplicate company id %q", c.ID)
		}
		l.Companies[c.ID] = c
	}
	return nil
}

func loadContracts(p *Paths, l *ledger.Ledger) error {
	var contracts []*model.Contract
	if err := readYAMLOptional(p.Contracts(), &contracts); err != nil {
		return err
	}
	for _, c := range contracts {
		if _, dup := l.Contracts[c.ID]; dup {
			return fmt.Errorf("duplicate contract id %q", c.ID)
		}
		l.Contracts[c.ID] = c
	}
	return nil
}

func loadInvoices(p *Paths, l *ledger.Ledger) error {
	dir := filepath.Join(p.Root(), invoicesDir)
	return walkShards(dir, func(path string) error {
		var invoices []*model.Invoice
		if err := readYAMLOptional(path, &invoices); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for _, inv := range invoices {
			if _, dup := l.Invoices[inv.ID]; dup {
				return fmt.Errorf("%s: duplicate invoice id %q", path, inv.ID)
			}
			l.Invoices[inv.ID] = inv
		}
		return nil
	})
}

func loadPayments(p *Paths, l *ledger.Ledger) error {
	dir := filepath.Join(p.Root(), paymentsDir)
	return walkShards(dir, func(path string) error {
		var payments []*model.Payment
		if err := readYAMLOptional(path, &payments); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for _, pay := range payments {
			if _, dup := l.Payments[pay.ID]; dup {
				return fmt.Errorf("%s: duplicate payment id %q", path, pay.ID)
			}
			l.Payments[pay.ID] = pay
		}
		return nil
	})
}

func walkShards(dir string, fn func(path string) error) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		if err := fn(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
