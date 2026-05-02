package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/vanducng/paymint/internal/core/config"
)

func newInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialise a paymint data directory.",
		Long: "Creates the data dir layout (.paymint/, companies.yaml stub) and writes\n" +
			"a config skeleton. Prompts for sheet ID, OAuth client ID, and your\n" +
			"issuer block (name + bank).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInit(cmd, force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing config (does not touch entity yaml)")
	return cmd
}

func runInit(cmd *cobra.Command, force bool) error {
	root, err := resolveDataDir(cmd)
	if err != nil {
		return err
	}
	files, err := newDataDirFiles(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(files.dotDir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", files.dotDir, err)
	}

	if _, err := os.Stat(files.configPath); err == nil && !force {
		return fmt.Errorf("config already exists at %s — pass --force to overwrite", files.configPath)
	}

	cmd.Printf("Initialising paymint data dir at %s\n", root)
	cmd.Println("Press Enter on prompts to skip; you can edit .paymint/config.yaml later.")

	reader := bufio.NewReader(cmd.InOrStdin())
	cfg := &config.Config{}
	cfg.SheetID = prompt(cmd.OutOrStdout(), reader, "Google Sheet ID")
	cfg.ClientID = prompt(cmd.OutOrStdout(), reader, "Google Cloud OAuth client ID")
	cfg.Issuer = config.Issuer{
		Name:    prompt(cmd.OutOrStdout(), reader, "Issuer name (you / your company)"),
		Address: prompt(cmd.OutOrStdout(), reader, "Issuer address (one line, optional)"),
		Email:   prompt(cmd.OutOrStdout(), reader, "Issuer email (optional)"),
		Bank: config.Bank{
			Name:          prompt(cmd.OutOrStdout(), reader, "Bank name"),
			AccountNumber: prompt(cmd.OutOrStdout(), reader, "Bank account number"),
			SWIFT:         prompt(cmd.OutOrStdout(), reader, "Bank SWIFT/BIC (optional)"),
			Address:       prompt(cmd.OutOrStdout(), reader, "Bank address (optional)"),
		},
	}

	if err := writeYAMLFile(files.configPath, cfg); err != nil {
		return err
	}
	if err := ensureMarker(files.markerPath); err != nil {
		return err
	}

	// Touch the entity stubs so a subsequent `list` doesn't trip on missing files.
	for _, p := range []string{files.yamlPaths.Companies(), files.yamlPaths.Contracts()} {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			if err := os.WriteFile(p, []byte("[]\n"), 0o600); err != nil {
				return err
			}
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "invoices"), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "payments"), 0o700); err != nil {
		return err
	}

	cmd.Printf("\nDone. Config: %s\n", files.configPath)
	cmd.Println("Next: paymint company add ...")
	return nil
}

// ensureMarker writes a UUID marker file the first time around. Sync code
// uses this to refuse running against a non-paymint directory (Red Team F9).
func ensureMarker(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	id := uuid.NewString()
	return os.WriteFile(path, []byte(id+"\n"), 0o600)
}

func prompt(w io.Writer, r *bufio.Reader, label string) string {
	_, _ = fmt.Fprintf(w, "%s: ", label)
	line, err := r.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimSpace(line)
}
