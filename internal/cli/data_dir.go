package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/vanducng/paymint/internal/store/yamlstore"
)

// envDataDir is the env var that overrides --data-dir.
const envDataDir = "PAYMINT_DATA_DIR"

// dotPaymint is the metadata subdir under every paymint data root. Holds
// config.yaml, pending.yaml, lock, and the marker UUID.
const dotPaymint = ".paymint"

// resolveDataDir returns the effective data dir, applying precedence
// flag > env > XDG_DATA_HOME/paymint > ~/.local/share/paymint.
func resolveDataDir(cmd *cobra.Command) (string, error) {
	if v, _ := cmd.Flags().GetString("data-dir"); v != "" {
		abs, err := filepath.Abs(v)
		return abs, err
	}
	if v := os.Getenv(envDataDir); v != "" {
		abs, err := filepath.Abs(v)
		return abs, err
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "paymint"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".local", "share", "paymint"), nil
}

// dataDirFiles bundles the well-known paths inside a paymint data dir.
type dataDirFiles struct {
	root        string
	dotDir      string
	configPath  string
	pendingPath string
	lockPath    string
	markerPath  string
	yamlPaths   *yamlstore.Paths
}

func newDataDirFiles(root string) (*dataDirFiles, error) {
	p, err := yamlstore.NewPaths(root)
	if err != nil {
		return nil, err
	}
	dot := filepath.Join(p.Root(), dotPaymint)
	return &dataDirFiles{
		root:        p.Root(),
		dotDir:      dot,
		configPath:  filepath.Join(dot, "config.yaml"),
		pendingPath: filepath.Join(dot, "pending.yaml"),
		lockPath:    filepath.Join(dot, "lock"),
		markerPath:  filepath.Join(dot, "marker"),
		yamlPaths:   p,
	}, nil
}

// requireInitialized errors out if the marker file is missing — preventing
// CLI commands from running against a non-paymint dir.
func (d *dataDirFiles) requireInitialized() error {
	if _, err := os.Stat(d.markerPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("data dir %q is not initialized — run 'paymint init' first", d.root)
		}
		return err
	}
	return nil
}
