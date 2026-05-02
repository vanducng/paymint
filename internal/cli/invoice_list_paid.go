package cli

import (
	"fmt"
	"sort"
	"strings"

	"cloud.google.com/go/civil"
	"github.com/spf13/cobra"

	"github.com/vanducng/paymint/internal/cli/output"
	"github.com/vanducng/paymint/internal/core/model"
	"github.com/vanducng/paymint/internal/core/money"
	"github.com/vanducng/paymint/internal/core/period"
	"github.com/vanducng/paymint/internal/store/pending"
)

// paymentID derives a kebab-safe payment ID from the invoice ID and a counter.
func paymentID(invoiceID string, idx int) string {
	return strings.ToLower(invoiceID) + fmt.Sprintf("-p%02d", idx)
}

func newInvoiceListCmd() *cobra.Command {
	var (
		monthStr, companyID, status string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List invoices.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openReadSession(cmd)
			if err != nil {
				return err
			}
			defer s.Close()

			var monthFilter *period.YearMonth
			if monthStr != "" {
				ym, err := period.Parse(monthStr)
				if err != nil {
					return fmt.Errorf("--month: %w", err)
				}
				monthFilter = &ym
			}
			ids := make([]string, 0, len(s.ledger.Invoices))
			for id, inv := range s.ledger.Invoices {
				if companyID != "" && inv.CompanyID != companyID {
					continue
				}
				if status != "" && string(inv.Status) != status {
					continue
				}
				if monthFilter != nil && period.FromDate(inv.IssueDate) != *monthFilter {
					continue
				}
				ids = append(ids, id)
			}
			sort.Strings(ids)
			rows := make([][]any, 0, len(ids))
			for _, id := range ids {
				inv := s.ledger.Invoices[id]
				paid := s.ledger.PaidCents(inv.ID, civil.Date{})
				outstanding := inv.TotalCents - paid
				rows = append(rows, []any{
					inv.ID, inv.CompanyID,
					inv.IssueDate.String(), inv.DueDate.String(),
					money.FormatUSD(inv.TotalCents),
					money.FormatUSD(outstanding),
					inv.Status,
				})
			}
			output.PrintTable(cmd.OutOrStdout(),
				[]any{"ID", "Company", "Issue", "Due", "Total", "Outstanding", "Status"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&monthStr, "month", "", "filter by issue month YYYY-MM")
	cmd.Flags().StringVar(&companyID, "company", "", "filter by company id")
	cmd.Flags().StringVar(&status, "status", "", "filter by status (issued|paid|overdue|revoked|draft)")
	return cmd
}

func newInvoiceMarkPaidCmd() *cobra.Command {
	var (
		dateStr, amountStr, method, reference string
	)
	cmd := &cobra.Command{
		Use:   "mark-paid <invoice-id>",
		Short: "Record a payment against an invoice.",
		Long: "Records a Payment against the invoice. Once Σpayments ≥ invoice.total\n" +
			"the invoice flips to 'paid'. Overpayment surfaces a warning but is\n" +
			"allowed — track refunds with a manually-edited Payment row in the sheet.",
		Args: cobra.ExactArgs(1),
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
			amount, err := money.ParseDollar(amountStr)
			if err != nil {
				return fmt.Errorf("--amount: %w", err)
			}

			payID := paymentID(invID, len(s.ledger.PaymentsFor(invID))+1)
			pay := &model.Payment{
				ID: payID, InvoiceID: invID,
				Date: date, AmountCents: amount,
				Method: method, Reference: reference,
			}
			if err := s.ledger.AddPayment(pay); err != nil {
				return err
			}

			payOp := pending.NewOp(pending.OpPaymentAdd, pay)
			if err := s.Save(payOp); err != nil {
				return err
			}
			cmd.Printf("recorded payment %s (%s) — queued op %s\n",
				pay.ID, money.FormatUSD(pay.AmountCents), payOp.OpID)

			// Recompute total paid and flip status if covered.
			totalPaid := s.ledger.PaidCents(invID, civil.Date{})
			switch {
			case totalPaid >= inv.TotalCents && inv.Status != model.StatusPaid:
				if err := s.ledger.MarkInvoiceStatus(invID, model.StatusPaid); err != nil {
					return err
				}
				statusOp := pending.NewOp(pending.OpInvoiceStatus, map[string]any{
					"invoice_id": invID, "status": string(model.StatusPaid),
				})
				if err := s.Save(statusOp); err != nil {
					return err
				}
				cmd.Printf("invoice %s now PAID — queued op %s\n", invID, statusOp.OpID)
				if totalPaid > inv.TotalCents {
					cmd.Printf("WARNING: overpayment of %s\n",
						money.FormatUSD(totalPaid-inv.TotalCents))
				}
			case totalPaid < inv.TotalCents:
				cmd.Printf("partial: %s of %s received (%s outstanding)\n",
					money.FormatUSD(totalPaid),
					money.FormatUSD(inv.TotalCents),
					money.FormatUSD(inv.TotalCents-totalPaid))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dateStr, "date", "", "payment date YYYY-MM-DD (required)")
	cmd.Flags().StringVar(&amountStr, "amount", "", "amount in USD (e.g. \"$1402.50\", required)")
	cmd.Flags().StringVar(&method, "method", "", "payment method (wire, check, etc.)")
	cmd.Flags().StringVar(&reference, "reference", "", "external reference (bank ref, wire id)")
	for _, f := range []string{"date", "amount"} {
		_ = cmd.MarkFlagRequired(f)
	}
	return cmd
}
