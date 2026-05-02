package cli

import (
	"github.com/spf13/cobra"

	"github.com/vanducng/paymint/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the paymint version, commit, and build date.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.Println(version.String())
			return nil
		},
	}
}
