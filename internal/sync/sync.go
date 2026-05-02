// Package sync orchestrates the sheet-canonical pull/push round-trip:
//
//	flock(.paymint/lock)
//	preDriveVer  := drive.GetVersion(spreadsheetID)
//	pull()                                  # 5 tabs → ledger → write dirty shards
//	push(pendingOps)                        # idempotent: skip ops already in sheet
//	postDriveVer := drive.GetVersion(spreadsheetID)
//	if pre != post: pull() ; push()         # bounded: max 2 retry cycles
//	git commit (only paths returned by yamlstore.Save)
//	release flock
package sync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/vanducng/paymint/internal/core/ledger"
	"github.com/vanducng/paymint/internal/drive"
	"github.com/vanducng/paymint/internal/sheets"
	"github.com/vanducng/paymint/internal/snapshot"
	"github.com/vanducng/paymint/internal/store/locks"
	"github.com/vanducng/paymint/internal/store/pending"
	"github.com/vanducng/paymint/internal/store/yamlstore"
)

// MaxRevisionRetries caps how many pull-push cycles we run when the Drive
// version moves under us (Red Team F11). Two is enough in practice; any more
// usually means a runaway loop and the user wants a clear failure.
const MaxRevisionRetries = 2

// Config bundles everything Sync needs.
type Config struct {
	SpreadsheetID string
	DataDir       string
	YAMLPaths     *yamlstore.Paths
	LockPath      string
	PendingPath   string

	Sheets sheets.Client
	Drive  drive.VersionGetter

	Logger io.Writer // optional; nil = silent
}

// Result summarises one sync run for the CLI.
type Result struct {
	PulledRows   int
	PushedOps    int
	WrittenPaths []string
	CommitMade   bool
	Retries      int
}

// Run executes one sync. The returned Result is non-nil even on partial
// failure (write paths up to the point of failure are reported).
func Run(ctx context.Context, cfg Config) (*Result, error) {
	unlock, err := locks.Acquire(cfg.LockPath, 10*time.Second)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unlock() }()

	res := &Result{}
	for attempt := 0; attempt <= MaxRevisionRetries; attempt++ {
		preVer, preErr := cfg.Drive.GetVersion(ctx, cfg.SpreadsheetID)
		if preErr != nil {
			// Drive may legitimately 404 when the user opened the sheet via a
			// sharing link but never added it to "My Drive". Sheets API can
			// still read/write the file. Degrade gracefully (Red Team F11 is
			// best-effort — concurrent-edit detection drops to "none" for
			// this run; surface the trade-off in setup docs).
			logf(cfg.Logger, "warn: drive version unavailable (%v) — concurrent-edit protection disabled\n", preErr)
		}

		// Pull the sheet into a fresh ledger, save dirty shards, capture paths.
		pulledLedger, pulled, written, err := pull(ctx, cfg)
		if err != nil {
			return res, err
		}
		res.PulledRows = pulled
		res.WrittenPaths = append(res.WrittenPaths, written...)

		// Drain pending against the now-current ledger.
		pushed, err := push(ctx, cfg, pulledLedger)
		if err != nil {
			return res, err
		}
		res.PushedOps += pushed

		if preErr != nil { // skip F11 retry loop if Drive is unreachable
			break
		}
		postVer, err := cfg.Drive.GetVersion(ctx, cfg.SpreadsheetID)
		if err != nil {
			return res, fmt.Errorf("drive version post: %w", err)
		}
		if postVer == preVer {
			break
		}
		res.Retries++
		if attempt == MaxRevisionRetries {
			return res, errors.New("sheet kept changing during sync — abort and retry manually")
		}
		logf(cfg.Logger, "sheet version changed during sync (pre=%d post=%d); retrying\n", preVer, postVer)
	}

	if err := commit(cfg, res); err != nil {
		return res, err
	}
	return res, nil
}

// commit snapshots the data dir if anything was written. CleanlinessCheck
// guards against accidentally committing user changes outside paymint's
// tracked dirs.
func commit(cfg Config, res *Result) error {
	if len(res.WrittenPaths) == 0 {
		return nil
	}
	repo, err := snapshot.New(cfg.DataDir)
	if err != nil {
		return err
	}
	if !repo.IsRepo() {
		logf(cfg.Logger, "skip commit: data dir is not a git repo (run 'paymint init --git-init' to enable)\n")
		return nil
	}
	if err := repo.CleanlinessCheck([]string{".paymint", "companies.yaml", "contracts.yaml", "invoices", "payments"}); err != nil {
		return err
	}
	msg := fmt.Sprintf("paymint sync: %d ops, %d shards", res.PushedOps, len(res.WrittenPaths))
	if err := repo.CommitPaths(msg, res.WrittenPaths); err != nil {
		return err
	}
	res.CommitMade = true
	return nil
}

// pull walks every tab, sanitises rows, and merges into a ledger. Returns
// the new ledger plus the count of pulled rows (header excluded).
func pull(ctx context.Context, cfg Config) (*ledger.Ledger, int, []string, error) {
	if err := cfg.Sheets.EnsureTabs(ctx, cfg.SpreadsheetID, sheets.PullOrder); err != nil {
		return nil, 0, nil, fmt.Errorf("ensure tabs: %w", err)
	}
	l := ledger.New()
	var totalRows int
	for _, tab := range sheets.PullOrder {
		header, ok := sheets.Headers[tab]
		if !ok {
			return nil, 0, nil, fmt.Errorf("unknown tab %q", tab)
		}
		if err := cfg.Sheets.EnsureHeader(ctx, cfg.SpreadsheetID, tab, header); err != nil {
			return nil, 0, nil, err
		}
		rows, err := cfg.Sheets.GetTab(ctx, cfg.SpreadsheetID, tab)
		if err != nil {
			return nil, 0, nil, err
		}
		// Skip header row.
		if len(rows) > 0 {
			rows = rows[1:]
		}
		for i, raw := range rows {
			clean, err := sheets.SanitizeRow(raw)
			if err != nil {
				return nil, 0, nil, fmt.Errorf("%s row %d: %w", tab, i+2, err)
			}
			if err := mergeRow(l, tab, clean); err != nil {
				return nil, 0, nil, fmt.Errorf("%s row %d: %w", tab, i+2, err)
			}
			totalRows++
		}
	}

	// Cross-validation catches dangling refs that the sheet might have.
	if err := l.CrossValidate(); err != nil {
		return nil, 0, nil, fmt.Errorf("cross-validate: %w", err)
	}

	// Mark *every* loaded entity dirty so the writer rewrites all shards. This
	// is the simplest safe approach for v0.1; later we can diff against the
	// previous local state.
	markAllDirty(l)
	written, err := yamlstore.Save(cfg.YAMLPaths, l)
	if err != nil {
		return nil, 0, nil, err
	}
	return l, totalRows, written, nil
}

// markAllDirty re-marks every entity so Save writes all shards. Done by
// using the public mutators in a no-op way (re-set status / re-add).
// The simplest path is to mark dirty months for invoices/payments via a
// direct status re-mark, and rely on AddCompany/AddContract having already
// dirtied flags during merge.
func markAllDirty(l *ledger.Ledger) {
	for id, inv := range l.Invoices {
		_ = l.MarkInvoiceStatus(id, inv.Status)
	}
}

// push iterates pending ops; for each, looks up the existing op_ids on the
// target tab and skips if already present (idempotency / F4). After all ops
// are flushed, pending.yaml is cleared.
func push(ctx context.Context, cfg Config, _ *ledger.Ledger) (int, error) {
	q, err := pending.Load(cfg.PendingPath)
	if err != nil {
		return 0, err
	}
	if len(q.Ops) == 0 {
		return 0, nil
	}

	existing, err := loadExistingOpIDs(ctx, cfg.Sheets, cfg.SpreadsheetID)
	if err != nil {
		return 0, err
	}

	pushed := 0
	for i, op := range q.Ops {
		if existing[op.OpID] {
			logf(cfg.Logger, "skip op %s — already in sheet\n", op.OpID)
			continue
		}
		row, tab, err := opToRow(op)
		if err != nil {
			return pushed, fmt.Errorf("op %s: %w", op.OpID, err)
		}
		if tab == "" { // status flips don't append; sheet pull will reflect them on the next run
			continue
		}
		if err := cfg.Sheets.AppendRows(ctx, cfg.SpreadsheetID, tab, [][]any{row}); err != nil {
			// Save the unprocessed tail back to pending so the next sync resumes.
			if saveErr := pending.Save(cfg.PendingPath, &pending.Queue{Ops: q.Ops[i:]}); saveErr != nil {
				return pushed, fmt.Errorf("append failed (%w); could not preserve pending: %v", err, saveErr)
			}
			return pushed, err
		}
		pushed++
	}
	// All ops landed: clear pending.
	if err := pending.Save(cfg.PendingPath, &pending.Queue{}); err != nil {
		return pushed, err
	}
	return pushed, nil
}

// loadExistingOpIDs reads the op_id column of every tab into a set.
func loadExistingOpIDs(ctx context.Context, c sheets.Client, spreadsheetID string) (map[string]bool, error) {
	out := make(map[string]bool)
	for _, tab := range sheets.PullOrder {
		rows, err := c.GetTab(ctx, spreadsheetID, tab)
		if err != nil {
			return nil, err
		}
		header, ok := sheets.Headers[tab]
		if !ok {
			continue
		}
		opIDCol := len(header) - 1 // last column
		for _, r := range rows {
			id := sheets.FieldString(r, opIDCol)
			if id != "" && id != "op_id" {
				out[id] = true
			}
		}
	}
	return out, nil
}

func logf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, format, args...)
}
