package cli

import (
	"github.com/spf13/cobra"

	"github.com/vanducng/paymint/internal/cli/output"
	"github.com/vanducng/paymint/internal/core/money"
)

func newPaymentCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "payment", Short: "Inspect payments."}
	cmd.AddCommand(newPaymentListCmd())
	return cmd
}

func newPaymentListCmd() *cobra.Command {
	var invoiceID string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List payments, optionally filtered to one invoice.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openReadSession(cmd)
			if err != nil {
				return err
			}
			defer s.Close()
			var rows [][]any
			if invoiceID != "" {
				for _, p := range s.ledger.PaymentsFor(invoiceID) {
					rows = append(rows, []any{
						p.ID, p.InvoiceID, p.Date.String(),
						money.FormatUSD(p.AmountCents), p.Method, p.Reference,
					})
				}
			} else {
				for _, p := range s.ledger.Payments {
					rows = append(rows, []any{
						p.ID, p.InvoiceID, p.Date.String(),
						money.FormatUSD(p.AmountCents), p.Method, p.Reference,
					})
				}
			}
			output.PrintTable(cmd.OutOrStdout(),
				[]any{"ID", "Invoice", "Date", "Amount", "Method", "Reference"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&invoiceID, "invoice", "", "filter to one invoice")
	return cmd
}
