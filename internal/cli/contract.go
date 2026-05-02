package cli

import (
	"fmt"
	"sort"

	"cloud.google.com/go/civil"
	"github.com/spf13/cobra"

	"github.com/vanducng/paymint/internal/cli/output"
	"github.com/vanducng/paymint/internal/core/model"
	"github.com/vanducng/paymint/internal/core/money"
	"github.com/vanducng/paymint/internal/store/pending"
)

func newContractCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "contract", Short: "Manage engagement contracts."}
	cmd.AddCommand(newContractAddCmd(), newContractListCmd())
	return cmd
}

func newContractAddCmd() *cobra.Command {
	var (
		id, companyID, title, rate, start, end, cadence, docURL, notes string
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new contract for a company.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openWriteSession(cmd)
			if err != nil {
				return err
			}
			defer s.Close()

			rateCents, err := money.ParseDollar(rate)
			if err != nil {
				return fmt.Errorf("--rate: %w", err)
			}
			startDate, err := civil.ParseDate(start)
			if err != nil {
				return fmt.Errorf("--start: %w", err)
			}
			var endDate civil.Date
			if end != "" {
				endDate, err = civil.ParseDate(end)
				if err != nil {
					return fmt.Errorf("--end: %w", err)
				}
			}
			contractID := id
			if contractID == "" {
				contractID = companyID + "-" + slugify(title)
			}
			c := &model.Contract{
				ID:          contractID,
				CompanyID:   companyID,
				Title:       title,
				DefaultRate: rateCents,
				Start:       startDate,
				End:         endDate,
				Cadence:     cadence,
				DocURL:      docURL,
				Notes:       notes,
			}
			if err := s.ledger.AddContract(c); err != nil {
				return err
			}
			op := pending.NewOp(pending.OpContractAdd, c)
			if err := s.Save(op); err != nil {
				return err
			}
			cmd.Printf("added contract %s (%s @ %s/hr) — queued op %s\n",
				c.ID, c.Title, money.FormatUSD(c.DefaultRate), op.OpID)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "contract id (default: <company>-<slugified-title>)")
	cmd.Flags().StringVar(&companyID, "company", "", "company id (required)")
	cmd.Flags().StringVar(&title, "title", "", "human-readable title (required)")
	cmd.Flags().StringVar(&rate, "rate", "", "default hourly rate in USD (e.g. \"$85.00\", required)")
	cmd.Flags().StringVar(&start, "start", "", "start date YYYY-MM-DD (required)")
	cmd.Flags().StringVar(&end, "end", "", "end date YYYY-MM-DD (optional, blank = open)")
	cmd.Flags().StringVar(&cadence, "cadence", "", "billing cadence label (e.g. \"monthly\")")
	cmd.Flags().StringVar(&docURL, "doc-url", "", "link to signed contract")
	cmd.Flags().StringVar(&notes, "notes", "", "free-form notes")
	for _, f := range []string{"company", "title", "rate", "start"} {
		_ = cmd.MarkFlagRequired(f)
	}
	return cmd
}

func newContractListCmd() *cobra.Command {
	var company string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List contracts.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openReadSession(cmd)
			if err != nil {
				return err
			}
			defer s.Close()
			ids := make([]string, 0, len(s.ledger.Contracts))
			for id, c := range s.ledger.Contracts {
				if company != "" && c.CompanyID != company {
					continue
				}
				ids = append(ids, id)
			}
			sort.Strings(ids)
			rows := make([][]any, 0, len(ids))
			for _, id := range ids {
				c := s.ledger.Contracts[id]
				endStr := "—"
				if (c.End != civil.Date{}) {
					endStr = c.End.String()
				}
				rows = append(rows, []any{
					c.ID, c.CompanyID, c.Title,
					money.FormatUSD(c.DefaultRate) + "/hr",
					c.Start.String(), endStr,
				})
			}
			output.PrintTable(cmd.OutOrStdout(),
				[]any{"ID", "Company", "Title", "Rate", "Start", "End"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&company, "company", "", "filter by company id")
	return cmd
}
