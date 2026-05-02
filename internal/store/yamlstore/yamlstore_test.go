package yamlstore_test

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vanducng/paymint/internal/core/ledger"
	"github.com/vanducng/paymint/internal/core/model"
	"github.com/vanducng/paymint/internal/store/yamlstore"
)

func seedABS(t *testing.T) *ledger.Ledger {
	t.Helper()
	l := ledger.New()
	require.NoError(t, l.AddCompany(&model.Company{
		ID: "abs", Slug: "abs", Name: "Adventure Bound Studio", Currency: "USD",
	}))
	require.NoError(t, l.AddContract(&model.Contract{
		ID: "abs-consulting", CompanyID: "abs", Title: "Consulting",
		DefaultRate: 8500,
		Start:       civil.Date{Year: 2026, Month: time.April, Day: 1},
	}))
	hours := []float32{4.0, 0.5, 3.0, 2.0, 4.0, 1.0, 1.5, 0.5}
	var lines []*model.InvoiceLine
	var total int64
	for i, h := range hours {
		amt := int64(math.Round(8500 * float64(h)))
		total += amt
		lines = append(lines, &model.InvoiceLine{
			ID:          "abs-l" + string(rune('a'+i)),
			InvoiceID:   "INV-abs-202604",
			Date:        civil.Date{Year: 2026, Month: time.April, Day: 1 + i},
			Description: "work item",
			Hours:       h,
			AmountCents: amt,
		})
	}
	require.NoError(t, l.AddInvoice(&model.Invoice{
		ID: "INV-abs-202604", CompanyID: "abs", ContractID: "abs-consulting",
		IssueDate:  civil.Date{Year: 2026, Month: time.April, Day: 2},
		DueDate:    civil.Date{Year: 2026, Month: time.April, Day: 17},
		TotalCents: total,
		Status:     model.StatusIssued,
		Lines:      lines,
	}))
	require.NoError(t, l.AddPayment(&model.Payment{
		ID: "pay-001", InvoiceID: "INV-abs-202604",
		Date:        civil.Date{Year: 2026, Month: time.April, Day: 20},
		AmountCents: 140250,
		Method:      "wire",
	}))
	return l
}

func TestSaveLoad_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	paths, err := yamlstore.NewPaths(dir)
	require.NoError(t, err)

	src := seedABS(t)
	written, err := yamlstore.Save(paths, src)
	require.NoError(t, err)
	assert.Len(t, written, 4) // companies, contracts, 1 invoice month, 1 payment month

	got, err := yamlstore.Load(paths)
	require.NoError(t, err)

	require.Len(t, got.Companies, 1)
	require.Len(t, got.Contracts, 1)
	require.Len(t, got.Invoices, 1)
	require.Len(t, got.Payments, 1)

	srcInv := src.Invoices["INV-abs-202604"]
	gotInv := got.Invoices["INV-abs-202604"]
	require.NotNil(t, gotInv)
	assert.Equal(t, srcInv.TotalCents, gotInv.TotalCents)
	assert.Equal(t, srcInv.IssueDate, gotInv.IssueDate)
	require.Len(t, gotInv.Lines, len(srcInv.Lines))
	for i, ln := range srcInv.Lines {
		assert.Equal(t, ln.Hours, gotInv.Lines[i].Hours, "line %d hours", i)
		assert.Equal(t, ln.AmountCents, gotInv.Lines[i].AmountCents, "line %d amount", i)
	}
}

func TestSave_OnlyDirtyShards(t *testing.T) {
	dir := t.TempDir()
	paths, err := yamlstore.NewPaths(dir)
	require.NoError(t, err)

	src := seedABS(t)
	_, err = yamlstore.Save(paths, src)
	require.NoError(t, err)

	// Capture mtimes of all shards.
	mtimeBefore := func(p string) time.Time {
		fi, err := os.Stat(p)
		require.NoError(t, err)
		return fi.ModTime()
	}
	companiesM := mtimeBefore(paths.Companies())
	// Use the canonical root the writer also uses (macOS /var → /private/var).
	invoiceShard := filepath.Join(paths.Root(), "invoices", "2026-04.yaml")
	invoicesM := mtimeBefore(invoiceShard)

	// Sleep 10ms to ensure mtime granularity.
	time.Sleep(10 * time.Millisecond)

	// Reload, mutate one invoice, save again. Only the invoices/2026-05.yaml
	// shard should change; companies.yaml stays untouched.
	loaded, err := yamlstore.Load(paths)
	require.NoError(t, err)
	require.NoError(t, loaded.MarkInvoiceStatus("INV-abs-202604", model.StatusPaid))
	written, err := yamlstore.Save(paths, loaded)
	require.NoError(t, err)
	assert.Equal(t, []string{invoiceShard}, written)

	// companies.yaml untouched.
	assert.Equal(t, companiesM, mtimeBefore(paths.Companies()))
	// invoices shard rewritten.
	assert.True(t, mtimeBefore(invoiceShard).After(invoicesM))
}

func TestLoad_DuplicateID_Fails(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "invoices"), 0o755))

	dup := []byte(`
- id: INV-abs-202604
  company_id: abs
  issue_date: 2026-04-02
  due_date: 2026-04-17
  total_cents: 0
  status: issued
- id: INV-abs-202604
  company_id: abs
  issue_date: 2026-04-02
  due_date: 2026-04-17
  total_cents: 0
  status: issued
`)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "invoices", "2026-04.yaml"), dup, 0o600))

	paths, err := yamlstore.NewPaths(dir)
	require.NoError(t, err)
	_, err = yamlstore.Load(paths)
	assert.ErrorContains(t, err, "duplicate invoice id")
}

func TestPaths_Within_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	paths, err := yamlstore.NewPaths(dir)
	require.NoError(t, err)

	for _, bad := range []string{
		filepath.Join(dir, "..", "escape.yaml"),
		"/etc/passwd",
	} {
		assert.Errorf(t, paths.Within(bad), "expected unsafe-path reject for %q", bad)
	}
	// Sanity: a normal path inside the root passes.
	assert.NoError(t, paths.Within(filepath.Join(dir, "ok", "shard.yaml")))
}
