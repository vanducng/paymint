// Package yamlstore is the on-disk persistence layer for Ledger. Files are
// monthly shards under a data root; loader walks them, writer touches only
// the months a Ledger has marked dirty.
//
// Layout:
//
//	<dataDir>/
//	  companies.yaml          (single file: array of Company)
//	  contracts.yaml          (single file: array of Contract)
//	  invoices/YYYY-MM.yaml   (one file per issue-month, contains invoices + lines)
//	  payments/YYYY-MM.yaml   (one file per payment-month)
package yamlstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vanducng/paymint/internal/core/period"
)

// osStat is a package-level alias to allow tests to swap in a fake stat if
// ever needed; production calls os.Stat directly.
var osStat = os.Stat

const (
	companiesFile = "companies.yaml"
	contractsFile = "contracts.yaml"
	invoicesDir   = "invoices"
	paymentsDir   = "payments"
)

// ErrUnsafePath is returned when a constructed path escapes the data root —
// either via traversal in an entity ID or through a symlink. See F7.
var ErrUnsafePath = errors.New("unsafe path")

// Paths resolves on-disk locations relative to a data root.
type Paths struct {
	root string // already absolute + symlink-evaluated
}

// NewPaths returns a Paths rooted at dataDir. The directory must exist; the
// constructor canonicalises and symlink-evaluates it once so every later
// resolution can re-assert the prefix cheaply.
func NewPaths(dataDir string) (*Paths, error) {
	if dataDir == "" {
		return nil, errors.New("data dir: empty")
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("data dir abs: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// Allow non-existent: caller may be creating the dir for the first time.
		// Fall back to abs path; subsequent resolutions enforce the prefix.
		resolved = abs
	}
	return &Paths{root: resolved}, nil
}

// Root returns the canonicalised data root.
func (p *Paths) Root() string { return p.root }

// Companies returns the companies file path.
func (p *Paths) Companies() string { return filepath.Join(p.root, companiesFile) }

// Contracts returns the contracts file path.
func (p *Paths) Contracts() string { return filepath.Join(p.root, contractsFile) }

// InvoicesShard returns the path of the YYYY-MM.yaml file under invoices/.
func (p *Paths) InvoicesShard(ym period.YearMonth) string {
	return filepath.Join(p.root, invoicesDir, ym.String()+".yaml")
}

// PaymentsShard returns the path of the YYYY-MM.yaml file under payments/.
func (p *Paths) PaymentsShard(ym period.YearMonth) string {
	return filepath.Join(p.root, paymentsDir, ym.String()+".yaml")
}

// Within asserts a candidate path stays beneath the data root after symlink
// resolution. Used by the writer right before opening a file for write.
//
// The candidate may not exist yet (writer creates it). We resolve symlinks
// against the deepest existing ancestor so `/var/...` vs `/private/var/...`
// (macOS) doesn't false-positive.
func (p *Paths) Within(candidate string) error {
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return fmt.Errorf("%w: abs(%s): %w", ErrUnsafePath, candidate, err)
	}
	resolved := resolveExisting(abs)
	rel, err := filepath.Rel(p.root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: %s escapes %s", ErrUnsafePath, resolved, p.root)
	}
	return nil
}

// resolveExisting walks upward from path until it finds an existing ancestor,
// evaluates its symlinks, then re-attaches the missing tail. Returns the
// original abs path on any failure (caller catches via Rel).
func resolveExisting(abs string) string {
	cur := abs
	var tail []string
	for {
		if _, err := osStat(cur); err == nil {
			resolved, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return abs
			}
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}
