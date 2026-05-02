package yamlstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	yaml "github.com/goccy/go-yaml"

	"github.com/vanducng/paymint/internal/core/ledger"
	"github.com/vanducng/paymint/internal/core/model"
	"github.com/vanducng/paymint/internal/core/period"
)

// Save writes only the shards that the ledger has marked dirty. After a
// successful write the ledger's dirty trackers are cleared.
//
// Returns the list of file paths actually written; sync code uses this to
// stage the right files in git (avoids `git add -A` per F9).
func Save(p *Paths, l *ledger.Ledger) ([]string, error) {
	d := l.Dirty()
	var written []string

	if d.Companies {
		path := p.Companies()
		if err := p.Within(path); err != nil {
			return written, err
		}
		if err := writeYAMLArray(path, sortedCompanies(l)); err != nil {
			return written, fmt.Errorf("write companies: %w", err)
		}
		written = append(written, path)
	}

	if d.Contracts {
		path := p.Contracts()
		if err := p.Within(path); err != nil {
			return written, err
		}
		if err := writeYAMLArray(path, sortedContracts(l)); err != nil {
			return written, fmt.Errorf("write contracts: %w", err)
		}
		written = append(written, path)
	}

	for _, ym := range d.InvoiceMonths {
		path := p.InvoicesShard(ym)
		if err := p.Within(path); err != nil {
			return written, err
		}
		shard := invoicesForMonth(l, ym)
		if err := writeOrRemove(path, shard); err != nil {
			return written, fmt.Errorf("write invoices %s: %w", ym, err)
		}
		written = append(written, path)
	}

	for _, ym := range d.PaymentMonths {
		path := p.PaymentsShard(ym)
		if err := p.Within(path); err != nil {
			return written, err
		}
		shard := paymentsForMonth(l, ym)
		if err := writeOrRemove(path, shard); err != nil {
			return written, fmt.Errorf("write payments %s: %w", ym, err)
		}
		written = append(written, path)
	}

	l.MarkClean()
	return written, nil
}

func writeYAMLArray(path string, items any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := yaml.MarshalWithOptions(items, yaml.Indent(2))
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// writeOrRemove writes the shard if it has rows; if it would be empty,
// removes the existing file (so empty months don't leave stale shards).
func writeOrRemove[T any](path string, items []T) error {
	if len(items) == 0 {
		err := os.Remove(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	return writeYAMLArray(path, items)
}

func sortedCompanies(l *ledger.Ledger) []*model.Company {
	out := make([]*model.Company, 0, len(l.Companies))
	for _, c := range l.Companies {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sortedContracts(l *ledger.Ledger) []*model.Contract {
	out := make([]*model.Contract, 0, len(l.Contracts))
	for _, c := range l.Contracts {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func invoicesForMonth(l *ledger.Ledger, ym period.YearMonth) []*model.Invoice {
	var out []*model.Invoice
	for _, inv := range l.Invoices {
		if period.FromDate(inv.IssueDate) == ym {
			out = append(out, inv)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IssueDate != out[j].IssueDate {
			return out[i].IssueDate.Before(out[j].IssueDate)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func paymentsForMonth(l *ledger.Ledger, ym period.YearMonth) []*model.Payment {
	var out []*model.Payment
	for _, p := range l.Payments {
		if period.FromDate(p.Date) == ym {
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
