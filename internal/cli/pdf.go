package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/vanducng/paymint/internal/core/ledger"
	"github.com/vanducng/paymint/internal/core/pdfdoc"
	"github.com/vanducng/paymint/internal/core/period"
	"github.com/vanducng/paymint/internal/pdf"
	"github.com/vanducng/paymint/internal/snapshot"
)

func newPDFCmd() *cobra.Command {
	var (
		invoiceID, monthStr, companyID, outDir string
		all                                    bool
	)
	cmd := &cobra.Command{
		Use:   "pdf",
		Short: "Render invoice(s) as PDF.",
		Long: "Renders one or more invoices to PDF.\n\n" +
			"Modes:\n" +
			"  --invoice <id>            single PDF\n" +
			"  --month YYYY-MM --all     every invoice issued in that month\n" +
			"  --month YYYY-MM --company X   one invoice for that company in that month",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openReadSession(cmd)
			if err != nil {
				return err
			}
			defer s.Close()
			cfg, err := loadCfg(s.files.configPath)
			if err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}

			ids, err := selectInvoices(s.ledger, invoiceID, monthStr, companyID, all)
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				return errors.New("no invoices match")
			}

			outRoot := outDir
			if outRoot == "" {
				outRoot = filepath.Join(s.files.root, "exports")
			}
			if err := os.MkdirAll(outRoot, 0o700); err != nil {
				return err
			}

			footer := pdfdoc.Footer{
				GeneratedAt: time.Now().UTC().Format(time.RFC3339),
				GitShortSHA: tryShortSHA(s.files.root),
			}

			for _, id := range ids {
				st, err := pdfdoc.BuildInvoice(s.ledger, id, cfg, footer)
				if err != nil {
					return err
				}
				out := filepath.Join(outRoot, id+".pdf")
				f, err := os.Create(out) //nolint:gosec // out is constructed from validated id
				if err != nil {
					return err
				}
				if err := pdf.Render(st, f); err != nil {
					_ = f.Close()
					return err
				}
				if err := f.Close(); err != nil {
					return err
				}
				cmd.Printf("wrote %s\n", out)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&invoiceID, "invoice", "", "render exactly this invoice id")
	cmd.Flags().StringVar(&monthStr, "month", "", "render invoices issued in YYYY-MM")
	cmd.Flags().StringVar(&companyID, "company", "", "filter by company id (combine with --month)")
	cmd.Flags().BoolVar(&all, "all", false, "match every company for the given month")
	cmd.Flags().StringVar(&outDir, "out", "", "output directory (default: <data-dir>/exports)")
	return cmd
}

// selectInvoices resolves the set of invoice IDs based on the flag combo.
func selectInvoices(l *ledger.Ledger, invoiceID, monthStr, companyID string, all bool) ([]string, error) {
	if invoiceID != "" {
		if _, ok := l.Invoices[invoiceID]; !ok {
			return nil, fmt.Errorf("invoice %q not found", invoiceID)
		}
		return []string{invoiceID}, nil
	}
	if monthStr == "" {
		return nil, errors.New("specify --invoice <id> OR --month YYYY-MM")
	}
	ym, err := period.Parse(monthStr)
	if err != nil {
		return nil, fmt.Errorf("--month: %w", err)
	}

	if companyID == "" && !all {
		return nil, errors.New("--month requires --company <id> or --all")
	}

	var ids []string
	for id, inv := range l.Invoices {
		if period.FromDate(inv.IssueDate) != ym {
			continue
		}
		if companyID != "" && inv.CompanyID != companyID {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// tryShortSHA returns the data dir's git short SHA, or "" if not a git repo.
func tryShortSHA(dataDir string) string {
	repo, err := snapshot.New(dataDir)
	if err != nil {
		return ""
	}
	sha, _ := repo.ShortSHA()
	return sha
}
