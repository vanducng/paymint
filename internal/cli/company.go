package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/vanducng/paymint/internal/cli/output"
	"github.com/vanducng/paymint/internal/core/model"
	"github.com/vanducng/paymint/internal/store/pending"
)

func newCompanyCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "company", Short: "Manage billable counterparties."}
	cmd.AddCommand(newCompanyAddCmd(), newCompanyListCmd())
	return cmd
}

func newCompanyAddCmd() *cobra.Command {
	var (
		slug, name, taxID, address, email string
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new company.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openWriteSession(cmd)
			if err != nil {
				return err
			}
			defer s.Close()
			id := strings.ToLower(slug)
			if id == "" {
				id = slugify(name)
			}
			c := &model.Company{
				ID: id, Slug: id, Name: name, Currency: "USD",
				TaxID: taxID, Address: address, Email: email,
			}
			if err := s.ledger.AddCompany(c); err != nil {
				return err
			}
			op := pending.NewOp(pending.OpCompanyAdd, c)
			if err := s.Save(op); err != nil {
				return err
			}
			cmd.Printf("added company %s (%s) — queued op %s\n", c.ID, c.Name, op.OpID)
			return nil
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "URL-safe slug (lowercase letters, digits, dashes; default: derived from name)")
	cmd.Flags().StringVar(&name, "name", "", "company display name (required)")
	cmd.Flags().StringVar(&taxID, "tax-id", "", "tax / VAT identifier")
	cmd.Flags().StringVar(&address, "address", "", "billing address")
	cmd.Flags().StringVar(&email, "email", "", "billing contact email")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newCompanyListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List companies.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := openReadSession(cmd)
			if err != nil {
				return err
			}
			defer s.Close()
			ids := make([]string, 0, len(s.ledger.Companies))
			for id := range s.ledger.Companies {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			rows := make([][]any, 0, len(ids))
			for _, id := range ids {
				c := s.ledger.Companies[id]
				rows = append(rows, []any{c.ID, c.Name, c.Currency, c.Email})
			}
			output.PrintTable(cmd.OutOrStdout(), []any{"ID", "Name", "Currency", "Email"}, rows)
			return nil
		},
	}
}

// slugify produces a kebab-case ID from an arbitrary display name. Falls
// back to "co" if the result is empty (caller usually catches via validation).
func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "co"
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}
