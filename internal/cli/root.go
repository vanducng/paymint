// Package cli wires the cobra command tree.
package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd returns the root paymint command with all subcommands attached.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "paymint",
		Short:         "Track received invoices, payments, and contracts.",
		Long:          "paymint is a CLI for tracking received invoices/contracts/payments,\nsyncing to a Google Sheet, snapshotting to git, and exporting per-month\nper-company PDF statements.",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.PersistentFlags().String("data-dir", "",
		"data directory (default: $PAYMINT_DATA_DIR > $XDG_DATA_HOME/paymint > ~/.local/share/paymint)")

	root.AddCommand(
		newVersionCmd(),
		newInitCmd(),
		newCompanyCmd(),
		newContractCmd(),
		newInvoiceCmd(),
		newPaymentCmd(),
		newAuthCmd(),
		newSyncCmd(),
		newPDFCmd(),
	)
	return root
}
