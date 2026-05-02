// Package pending implements the local op queue that the CLI writes into and
// the sync code drains. Each op carries a UUIDv4 op_id used for idempotent
// push (the sheet stores op_id per row; sync skips ops whose id already
// landed). See plan.md → Phase 3 / Phase 4 for the round-trip contract.
package pending

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	yaml "github.com/goccy/go-yaml"
	"github.com/google/uuid"
)

// OpKind enumerates the supported pending operation types. Sync walks ops in
// dependency order (Company → Contract → Invoice → Payment), so adding a new
// kind requires updating the order constants in the sync package too.
type OpKind string

// Op kinds.
const (
	OpCompanyAdd     OpKind = "company_add"
	OpContractAdd    OpKind = "contract_add"
	OpInvoiceAdd     OpKind = "invoice_add"
	OpInvoiceLineAdd OpKind = "invoice_line_add"
	OpInvoiceLineRm  OpKind = "invoice_line_remove"
	OpPaymentAdd     OpKind = "payment_add"
	OpInvoiceStatus  OpKind = "invoice_status"
)

// Op is one queued mutation. Payload is intentionally `any` so YAML decode
// preserves the row shape (sync code casts to a typed view per Kind).
type Op struct {
	OpID     string    `yaml:"op_id"`
	Kind     OpKind    `yaml:"kind"`
	QueuedAt time.Time `yaml:"queued_at"`
	Payload  any       `yaml:"payload"`
}

// Queue is the in-memory representation of pending.yaml.
type Queue struct {
	Ops []Op `yaml:"ops"`
}

// NewOp returns an Op with a freshly minted UUID and the current UTC time.
func NewOp(kind OpKind, payload any) Op {
	return Op{
		OpID:     uuid.NewString(),
		Kind:     kind,
		QueuedAt: time.Now().UTC(),
		Payload:  payload,
	}
}

// Load reads pending.yaml; missing file is tolerated (returns empty queue).
func Load(path string) (*Queue, error) {
	b, err := os.ReadFile(path) //nolint:gosec // path comes from validated paths
	if errors.Is(err, fs.ErrNotExist) {
		return &Queue{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return &Queue{}, nil
	}
	var q Queue
	if err := yaml.Unmarshal(b, &q); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &q, nil
}

// Save writes the queue atomically (tmp + rename, mode 0600).
func Save(path string, q *Queue) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := yaml.MarshalWithOptions(q, yaml.Indent(2))
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Append loads the queue, adds op, and saves.
func Append(path string, op Op) error {
	q, err := Load(path)
	if err != nil {
		return err
	}
	q.Ops = append(q.Ops, op)
	return Save(path, q)
}
