package sync_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vanducng/paymint/internal/core/model"
	pkgsheets "github.com/vanducng/paymint/internal/sheets"
	"github.com/vanducng/paymint/internal/store/pending"
	"github.com/vanducng/paymint/internal/store/yamlstore"
	syncpkg "github.com/vanducng/paymint/internal/sync"
)

// fakeSheets implements sheets.Client in-memory.
type fakeSheets struct {
	mu   sync.Mutex
	tabs map[string][][]any
}

func newFakeSheets() *fakeSheets {
	f := &fakeSheets{tabs: map[string][][]any{}}
	for tab, h := range pkgsheets.Headers {
		header := make([]any, len(h))
		for i, s := range h {
			header[i] = s
		}
		f.tabs[tab] = [][]any{header}
	}
	return f
}

func (f *fakeSheets) EnsureTabs(_ context.Context, _ string, _ []string) error { return nil }

func (f *fakeSheets) GetTab(_ context.Context, _ string, tab string) ([][]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]any, len(f.tabs[tab]))
	copy(out, f.tabs[tab])
	return out, nil
}

func (f *fakeSheets) EnsureHeader(_ context.Context, _ string, tab string, header []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.tabs[tab]) == 0 {
		row := make([]any, len(header))
		for i, h := range header {
			row[i] = h
		}
		f.tabs[tab] = [][]any{row}
	}
	return nil
}

func (f *fakeSheets) AppendRows(_ context.Context, _ string, tab string, rows [][]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tabs[tab] = append(f.tabs[tab], rows...)
	return nil
}

func (f *fakeSheets) UpdateRow(_ context.Context, _ string, tab string, idx int, row []any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	rows := f.tabs[tab]
	for len(rows) <= idx {
		rows = append(rows, nil)
	}
	rows[idx] = row
	f.tabs[tab] = rows
	return nil
}

// fakeDrive returns whatever Version was set, allowing tests to simulate
// concurrent edits by changing the version mid-run.
type fakeDrive struct {
	calls   int
	mu      sync.Mutex
	version int64
}

func (d *fakeDrive) GetVersion(context.Context, string) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	return d.version, nil
}

func TestSync_PushPullRoundTrip(t *testing.T) {
	dir := t.TempDir()
	paths, err := yamlstore.NewPaths(dir)
	require.NoError(t, err)

	pendingPath := filepath.Join(dir, ".paymint", "pending.yaml")
	lockPath := filepath.Join(dir, ".paymint", "lock")

	// Seed pending with a company + a contract + an invoice + a line.
	companyOp := pending.NewOp(pending.OpCompanyAdd, &model.Company{
		ID: "abs", Slug: "abs", Name: "ABS", Currency: "USD",
	})
	contractOp := pending.NewOp(pending.OpContractAdd, &model.Contract{
		ID: "abs-c", CompanyID: "abs", Title: "Consulting",
		DefaultRate: 8500,
		Start:       civil.Date{Year: 2026, Month: 4, Day: 1},
	})
	invoiceOp := pending.NewOp(pending.OpInvoiceAdd, &model.Invoice{
		ID: "INV-abs-202604", CompanyID: "abs", ContractID: "abs-c",
		IssueDate:  civil.Date{Year: 2026, Month: 4, Day: 2},
		DueDate:    civil.Date{Year: 2026, Month: 4, Day: 17},
		TotalCents: 34000,
		Status:     model.StatusIssued,
	})
	lineOp := pending.NewOp(pending.OpInvoiceLineAdd, &model.InvoiceLine{
		ID: "inv-abs-202604-l01", InvoiceID: "INV-abs-202604",
		Date:        civil.Date{Year: 2026, Month: 4, Day: 2},
		Description: "Explore",
		Hours:       4.0, AmountCents: 34000,
	})
	for _, op := range []pending.Op{companyOp, contractOp, invoiceOp, lineOp} {
		require.NoError(t, pending.Append(pendingPath, op))
	}

	fs := newFakeSheets()
	fd := &fakeDrive{version: 1}

	res, err := syncpkg.Run(context.Background(), syncpkg.Config{
		SpreadsheetID: "fake",
		DataDir:       paths.Root(),
		YAMLPaths:     paths,
		LockPath:      lockPath,
		PendingPath:   pendingPath,
		Sheets:        fs,
		Drive:         fd,
	})
	require.NoError(t, err)
	assert.Equal(t, 4, res.PushedOps)
	assert.Equal(t, 0, res.Retries)

	// Pending should now be empty.
	q, err := pending.Load(pendingPath)
	require.NoError(t, err)
	assert.Empty(t, q.Ops)

	// Sheet should hold one data row per tab (header + 1).
	for tab, want := range map[string]int{
		pkgsheets.TabCompanies: 2, pkgsheets.TabContracts: 2,
		pkgsheets.TabInvoices: 2, pkgsheets.TabInvoiceLines: 2,
	} {
		rows, _ := fs.GetTab(context.Background(), "fake", tab)
		assert.Lenf(t, rows, want, "tab %s row count", tab)
	}
}

func TestSync_OpIDIdempotency(t *testing.T) {
	dir := t.TempDir()
	paths, err := yamlstore.NewPaths(dir)
	require.NoError(t, err)
	pendingPath := filepath.Join(dir, ".paymint", "pending.yaml")
	lockPath := filepath.Join(dir, ".paymint", "lock")

	// Pre-populate the sheet with a company row carrying op_id "op-already".
	fs := newFakeSheets()
	fs.tabs[pkgsheets.TabCompanies] = append(
		fs.tabs[pkgsheets.TabCompanies],
		[]any{"abs", "abs", "ABS", "USD", "", "", "", "op-already"})

	// Queue a pending op with the SAME op_id — sync should skip the append.
	op := pending.Op{
		OpID: "op-already", Kind: pending.OpCompanyAdd, QueuedAt: time.Now(),
		Payload: &model.Company{ID: "abs", Slug: "abs", Name: "ABS", Currency: "USD"},
	}
	require.NoError(t, pending.Append(pendingPath, op))

	fd := &fakeDrive{version: 1}
	res, err := syncpkg.Run(context.Background(), syncpkg.Config{
		SpreadsheetID: "fake",
		DataDir:       paths.Root(),
		YAMLPaths:     paths,
		LockPath:      lockPath,
		PendingPath:   pendingPath,
		Sheets:        fs,
		Drive:         fd,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, res.PushedOps)

	// Sheet should still have one data row, not two.
	rows, _ := fs.GetTab(context.Background(), "fake", pkgsheets.TabCompanies)
	assert.Len(t, rows, 2) // header + 1
}

func TestSync_RevisionRetry(t *testing.T) {
	dir := t.TempDir()
	paths, err := yamlstore.NewPaths(dir)
	require.NoError(t, err)
	pendingPath := filepath.Join(dir, ".paymint", "pending.yaml")
	lockPath := filepath.Join(dir, ".paymint", "lock")

	// fakeDrive bumps version on the first call, stable thereafter -> exactly one retry.
	fd := &bumpingDrive{}
	fs := newFakeSheets()
	res, err := syncpkg.Run(context.Background(), syncpkg.Config{
		SpreadsheetID: "fake",
		DataDir:       paths.Root(),
		YAMLPaths:     paths,
		LockPath:      lockPath,
		PendingPath:   pendingPath,
		Sheets:        fs,
		Drive:         fd,
	})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, res.Retries, 1)
}

// bumpingDrive returns version=1 then version=2 (concurrent edit) then 2,2... so
// the first iteration sees a mismatch but the retry succeeds.
type bumpingDrive struct {
	calls int
	mu    sync.Mutex
}

func (d *bumpingDrive) GetVersion(context.Context, string) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	switch d.calls {
	case 1:
		return 1, nil
	case 2:
		return 2, nil
	default:
		return 2, nil
	}
}
