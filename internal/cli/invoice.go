package cli

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"cloud.google.com/go/civil"
	"github.com/spf13/cobra"

	"github.com/vanducng/paymint/internal/core/model"
	"github.com/vanducng/paymint/internal/core/money"
	"github.com/vanducng/paymint/internal/store/pending"
)

func newInvoiceCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "invoice", Short: "Manage invoices."}
	cmd.AddCommand(
		newInvoiceAddCmd(),
		newInvoiceListCmd(),
		newInvoiceMarkPaidCmd(),
		newInvoiceLineCmd(),
	)
	return cmd
}

func newInvoiceAddCmd() *cobra.Command {
	var (
		companyID, contractID, issue, due, notes string
		lineFlags                                []string
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new invoice with one or more line items.",
		Long: "Invoice ID is auto-derived as INV-<company-slug>-<YYYYMM> from the\n" +
			"company and the issue date. Adding a second invoice for the same\n" +
			"(company, month) is rejected.\n\n" +
			"Repeat --line for each entry. Format:\n" +
			"  --line \"YYYY-MM-DD,description,hours[,rate][,ref]\"",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openWriteSession(cmd)
			if err != nil {
				return err
			}
			defer s.Close()

			company, ok := s.ledger.Companies[companyID]
			if !ok {
				return fmt.Errorf("unknown company %q (run 'paymint company list')", companyID)
			}
			issueDate, err := civil.ParseDate(issue)
			if err != nil {
				return fmt.Errorf("--issue: %w", err)
			}
			dueDate, err := civil.ParseDate(due)
			if err != nil {
				return fmt.Errorf("--due: %w", err)
			}

			invID := model.MakeInvoiceID(company.Slug, issueDate.Year, int(issueDate.Month))

			var contractRate int64
			if contractID == "" {
				// Pick a default contract if exactly one matches.
				if id, rate := singleContractRate(s, companyID); id != "" {
					contractID = id
					contractRate = rate
				}
			} else {
				ct, ok := s.ledger.Contracts[contractID]
				if !ok {
					return fmt.Errorf("unknown contract %q", contractID)
				}
				contractRate = ct.DefaultRate
			}

			lines, total, err := buildLines(invID, lineFlags, contractRate)
			if err != nil {
				return err
			}

			inv := &model.Invoice{
				ID:         invID,
				CompanyID:  companyID,
				ContractID: contractID,
				IssueDate:  issueDate,
				DueDate:    dueDate,
				TotalCents: total,
				Status:     model.StatusIssued,
				Notes:      notes,
				Lines:      lines,
			}
			if err := s.ledger.AddInvoice(inv); err != nil {
				return err
			}
			op := pending.NewOp(pending.OpInvoiceAdd, inv)
			if err := s.Save(op); err != nil {
				return err
			}
			cmd.Printf("added invoice %s (%d lines, total %s) — queued op %s\n",
				inv.ID, len(inv.Lines), money.FormatUSD(inv.TotalCents), op.OpID)
			return nil
		},
	}
	cmd.Flags().StringVar(&companyID, "company", "", "company id (required)")
	cmd.Flags().StringVar(&contractID, "contract", "", "contract id (auto if exactly one for company)")
	cmd.Flags().StringVar(&issue, "issue", "", "issue date YYYY-MM-DD (required)")
	cmd.Flags().StringVar(&due, "due", "", "due date YYYY-MM-DD (required)")
	cmd.Flags().StringVar(&notes, "notes", "", "free-form notes")
	cmd.Flags().StringArrayVar(&lineFlags, "line", nil,
		"hourly entry: \"date,description,hours[,rate][,ref]\" — repeat per line")
	for _, f := range []string{"company", "issue", "due", "line"} {
		_ = cmd.MarkFlagRequired(f)
	}
	return cmd
}

// singleContractRate returns the default rate of a company's only active
// contract, or ("", 0) if there are 0 or 2+ matches.
func singleContractRate(s *session, companyID string) (string, int64) {
	var matched *model.Contract
	for _, c := range s.ledger.Contracts {
		if c.CompanyID != companyID {
			continue
		}
		if matched != nil {
			return "", 0
		}
		matched = c
	}
	if matched == nil {
		return "", 0
	}
	return matched.ID, matched.DefaultRate
}

// buildLines parses --line flags into InvoiceLine structs and computes the
// running total. Format: "date,description,hours[,rate][,ref]". Rate accepts
// the money.ParseDollar grammar; if blank, falls back to fallbackRate.
func buildLines(invID string, raw []string, fallbackRate int64) ([]*model.InvoiceLine, int64, error) {
	var (
		lines []*model.InvoiceLine
		total int64
	)
	for i, s := range raw {
		parts := splitCSV(s, 5)
		if len(parts) < 3 {
			return nil, 0, fmt.Errorf("--line[%d]: need at least date,description,hours", i)
		}
		date, err := civil.ParseDate(parts[0])
		if err != nil {
			return nil, 0, fmt.Errorf("--line[%d] date: %w", i, err)
		}
		hours, err := strconv.ParseFloat(parts[2], 32)
		if err != nil || hours <= 0 {
			return nil, 0, fmt.Errorf("--line[%d] hours: must be positive number, got %q", i, parts[2])
		}
		rateCents := int64(0)
		if len(parts) >= 4 && parts[3] != "" {
			rateCents, err = money.ParseDollar(parts[3])
			if err != nil {
				return nil, 0, fmt.Errorf("--line[%d] rate: %w", i, err)
			}
		}
		ref := ""
		if len(parts) >= 5 {
			ref = parts[4]
		}
		effective := rateCents
		if effective == 0 {
			effective = fallbackRate
		}
		if effective == 0 {
			return nil, 0, fmt.Errorf("--line[%d]: no rate specified and contract has no default", i)
		}
		amount := int64(math.Round(float64(effective) * hours))
		total += amount
		lines = append(lines, &model.InvoiceLine{
			ID:          lineID(invID, i+1),
			InvoiceID:   invID,
			Date:        date,
			Description: parts[1],
			Ref:         ref,
			RateCents:   rateCents,
			Hours:       float32(hours),
			AmountCents: amount,
		})
	}
	return lines, total, nil
}

// splitCSV splits a CSV-ish string into up to maxFields parts. Quoting is
// not supported (descriptions with commas should be quoted at the shell).
func splitCSV(s string, maxFields int) []string {
	out := strings.SplitN(s, ",", maxFields)
	for i, v := range out {
		out[i] = strings.TrimSpace(v)
	}
	return out
}

// lineID builds a kebab-case line ID. Invoice IDs are mixed-case
// (INV-abs-202604) but model.ValidateID requires lowercase, so we down-case
// the prefix before appending the line index.
func lineID(invoiceID string, idx int) string {
	return strings.ToLower(invoiceID) + fmt.Sprintf("-l%02d", idx)
}
