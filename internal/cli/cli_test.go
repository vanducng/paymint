package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vanducng/paymint/internal/cli"
	"github.com/vanducng/paymint/internal/store/pending"
)

// runCmd executes a paymint subcommand with stdout captured. stdin can be
// pre-loaded for the init prompt flow.
func runCmd(t *testing.T, dataDir string, stdin string, args ...string) (string, error) {
	t.Helper()
	root := cli.NewRootCmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	if stdin != "" {
		root.SetIn(strings.NewReader(stdin))
	}
	root.SetArgs(append([]string{"--data-dir", dataDir}, args...))
	err := root.Execute()
	if err != nil {
		out.WriteString(err.Error())
	}
	return out.String(), err
}

func TestHappyPath_AddInvoiceMarkPaid(t *testing.T) {
	dir := t.TempDir()

	// 9 newlines satisfy the 9 prompts in `paymint init`.
	_, err := runCmd(t, dir, strings.Repeat("\n", 9), "init")
	require.NoError(t, err)

	_, err = runCmd(t, dir, "", "company", "add", "--slug", "abs", "--name", "ABS")
	require.NoError(t, err)

	_, err = runCmd(t, dir, "",
		"contract", "add", "--company", "abs", "--title", "Consulting",
		"--rate", "$85.00", "--start", "2026-04-01")
	require.NoError(t, err)

	_, err = runCmd(t, dir, "",
		"invoice", "add", "--company", "abs",
		"--issue", "2026-04-02", "--due", "2026-04-17",
		"--line", "2026-04-02,Explore the API,4",
		"--line", "2026-04-04,Standup,0.5")
	require.NoError(t, err)

	listOut, err := runCmd(t, dir, "", "invoice", "list")
	require.NoError(t, err)
	assert.Contains(t, listOut, "INV-abs-202604")
	assert.Contains(t, listOut, "$382.50") // 4.5 * $85

	_, err = runCmd(t, dir, "",
		"invoice", "mark-paid", "INV-abs-202604",
		"--date", "2026-04-20", "--amount", "$382.50", "--method", "wire")
	require.NoError(t, err)

	listOut, err = runCmd(t, dir, "", "invoice", "list")
	require.NoError(t, err)
	assert.Contains(t, listOut, "paid")

	// Pending queue should hold every op_id from the run.
	q, err := pending.Load(filepath.Join(dir, ".paymint", "pending.yaml"))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(q.Ops), 5) // company + contract + invoice + payment + status
	for _, op := range q.Ops {
		assert.NotEmpty(t, op.OpID)
		assert.NotEmpty(t, op.Kind)
	}
}

func TestInvoiceAdd_RejectsDuplicateMonth(t *testing.T) {
	dir := t.TempDir()
	_, err := runCmd(t, dir, strings.Repeat("\n", 9), "init")
	require.NoError(t, err)
	_, err = runCmd(t, dir, "", "company", "add", "--slug", "abs", "--name", "ABS")
	require.NoError(t, err)
	_, err = runCmd(t, dir, "",
		"contract", "add", "--company", "abs", "--title", "C",
		"--rate", "$50", "--start", "2026-04-01")
	require.NoError(t, err)
	_, err = runCmd(t, dir, "",
		"invoice", "add", "--company", "abs",
		"--issue", "2026-04-02", "--due", "2026-04-17",
		"--line", "2026-04-02,Work,1")
	require.NoError(t, err)

	out, err := runCmd(t, dir, "",
		"invoice", "add", "--company", "abs",
		"--issue", "2026-04-15", "--due", "2026-04-30",
		"--line", "2026-04-15,More,2")
	assert.Error(t, err)
	assert.Contains(t, out, "already exists")
}

func TestRequireInitialized(t *testing.T) {
	dir := t.TempDir()
	// Skip init — any data command should refuse.
	out, err := runCmd(t, dir, "", "company", "list")
	assert.Error(t, err)
	assert.Contains(t, out, "is not initialized")
	// Sanity: marker is still absent.
	_, statErr := os.Stat(filepath.Join(dir, ".paymint", "marker"))
	assert.True(t, os.IsNotExist(statErr))
}
