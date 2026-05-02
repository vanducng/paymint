package pending_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vanducng/paymint/internal/store/pending"
)

func TestQueue_LoadEmptyMissing(t *testing.T) {
	q, err := pending.Load(filepath.Join(t.TempDir(), "missing.yaml"))
	require.NoError(t, err)
	assert.Empty(t, q.Ops)
}

func TestQueue_AppendRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.yaml")
	op1 := pending.NewOp(pending.OpCompanyAdd, map[string]string{"slug": "abs"})
	op2 := pending.NewOp(pending.OpInvoiceAdd, map[string]string{"id": "INV-abs-202604"})
	require.NoError(t, pending.Append(path, op1))
	require.NoError(t, pending.Append(path, op2))

	q, err := pending.Load(path)
	require.NoError(t, err)
	require.Len(t, q.Ops, 2)
	assert.Equal(t, op1.OpID, q.Ops[0].OpID)
	assert.Equal(t, pending.OpCompanyAdd, q.Ops[0].Kind)
	assert.Equal(t, op2.OpID, q.Ops[1].OpID)
}

func TestNewOp_GeneratesUUIDs(t *testing.T) {
	a := pending.NewOp(pending.OpInvoiceAdd, nil)
	b := pending.NewOp(pending.OpInvoiceAdd, nil)
	assert.NotEqual(t, a.OpID, b.OpID)
	assert.Len(t, a.OpID, 36) // UUIDv4 canonical form
}
