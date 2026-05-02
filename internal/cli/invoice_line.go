package cli

import (
	"fmt"
	"math"

	"cloud.google.com/go/civil"
	"github.com/spf13/cobra"

	"github.com/vanducng/paymint/internal/core/model"
	"github.com/vanducng/paymint/internal/core/money"
	"github.com/vanducng/paymint/internal/store/pending"
)

func newInvoiceLineCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "line", Short: "Edit invoice lines after creation."}
	cmd.AddCommand(newInvoiceLineAddCmd(), newInvoiceLineRemoveCmd())
	return cmd
}

func newInvoiceLineAddCmd() *cobra.Command {
	var (
		dateStr, desc, ref, rateStr string
		hours                       float64
	)
	cmd := &cobra.Command{
		Use:   "add <invoice-id>",
		Short: "Append a new line to an existing invoice.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openWriteSession(cmd)
			if err != nil {
				return err
			}
			defer s.Close()
			invID := args[0]
			inv, ok := s.ledger.Invoices[invID]
			if !ok {
				return fmt.Errorf("invoice %q not found", invID)
			}
			date, err := civil.ParseDate(dateStr)
			if err != nil {
				return fmt.Errorf("--date: %w", err)
			}
			if hours <= 0 {
				return fmt.Errorf("--hours: must be > 0")
			}
			rateCents := int64(0)
			if rateStr != "" {
				rateCents, err = money.ParseDollar(rateStr)
				if err != nil {
					return fmt.Errorf("--rate: %w", err)
				}
			}
			fallback := contractRateFor(s, inv.ContractID)
			effective := rateCents
			if effective == 0 {
				effective = fallback
			}
			if effective == 0 {
				return fmt.Errorf("no rate: line lacks --rate and contract has no default")
			}
			line := &model.InvoiceLine{
				ID:          lineID(invID, len(inv.Lines)+1),
				InvoiceID:   invID,
				Date:        date,
				Description: desc,
				Ref:         ref,
				RateCents:   rateCents,
				Hours:       float32(hours),
				AmountCents: int64(math.Round(float64(effective) * hours)),
			}
			inv.Lines = append(inv.Lines, line)
			inv.TotalCents += line.AmountCents
			// Re-validate to catch any user mistake.
			if err := inv.Validate(s.ledger.Companies[inv.CompanyID].Slug, fallback); err != nil {
				return err
			}
			// Mark dirty by going through MarkInvoiceStatus (no status change but
			// dirties the month) — simpler than exposing another mutator.
			if err := s.ledger.MarkInvoiceStatus(invID, inv.Status); err != nil {
				return err
			}
			op := pending.NewOp(pending.OpInvoiceLineAdd, line)
			if err := s.Save(op); err != nil {
				return err
			}
			cmd.Printf("added line %s (%s) — queued op %s\n",
				line.ID, money.FormatUSD(line.AmountCents), op.OpID)
			return nil
		},
	}
	cmd.Flags().StringVar(&dateStr, "date", "", "line date YYYY-MM-DD (required)")
	cmd.Flags().StringVar(&desc, "description", "", "line description (required)")
	cmd.Flags().StringVar(&ref, "ref", "", "external reference (jira/meeting)")
	cmd.Flags().Float64Var(&hours, "hours", 0, "hours billed (required, > 0)")
	cmd.Flags().StringVar(&rateStr, "rate", "", "override rate (e.g. \"$95.00\")")
	for _, f := range []string{"date", "description", "hours"} {
		_ = cmd.MarkFlagRequired(f)
	}
	return cmd
}

func newInvoiceLineRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <invoice-id> <line-id>",
		Short: "Remove a line from an invoice.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := openWriteSession(cmd)
			if err != nil {
				return err
			}
			defer s.Close()
			invID, lineID := args[0], args[1]
			inv, ok := s.ledger.Invoices[invID]
			if !ok {
				return fmt.Errorf("invoice %q not found", invID)
			}
			idx := -1
			for i, l := range inv.Lines {
				if l.ID == lineID {
					idx = i
					break
				}
			}
			if idx < 0 {
				return fmt.Errorf("line %q not found on invoice %s", lineID, invID)
			}
			removed := inv.Lines[idx]
			inv.Lines = append(inv.Lines[:idx], inv.Lines[idx+1:]...)
			inv.TotalCents -= removed.AmountCents
			if err := s.ledger.MarkInvoiceStatus(invID, inv.Status); err != nil {
				return err
			}
			op := pending.NewOp(pending.OpInvoiceLineRm, map[string]any{
				"invoice_id": invID, "line_id": lineID,
			})
			if err := s.Save(op); err != nil {
				return err
			}
			cmd.Printf("removed line %s — queued op %s\n", lineID, op.OpID)
			return nil
		},
	}
	return cmd
}

// contractRateFor returns the default rate for an invoice's contract, or 0 if
// the invoice has no contract or the contract is missing.
func contractRateFor(s *session, contractID string) int64 {
	if contractID == "" {
		return 0
	}
	c, ok := s.ledger.Contracts[contractID]
	if !ok {
		return 0
	}
	return c.DefaultRate
}
