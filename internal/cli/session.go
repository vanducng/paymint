package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/vanducng/paymint/internal/core/ledger"
	"github.com/vanducng/paymint/internal/store/locks"
	"github.com/vanducng/paymint/internal/store/pending"
	"github.com/vanducng/paymint/internal/store/yamlstore"
)

// session bundles a locked, loaded ledger with the data-dir paths so write
// commands don't have to thread state by hand.
type session struct {
	files  *dataDirFiles
	ledger *ledger.Ledger
	unlock func() error
}

// openWriteSession resolves the data dir, asserts initialization, takes the
// .paymint/lock, loads the ledger, and returns a session ready for mutation.
// Call session.Close() (typically deferred) to release the lock.
func openWriteSession(cmd *cobra.Command) (*session, error) {
	root, err := resolveDataDir(cmd)
	if err != nil {
		return nil, err
	}
	files, err := newDataDirFiles(root)
	if err != nil {
		return nil, err
	}
	if err := files.requireInitialized(); err != nil {
		return nil, err
	}
	unlock, err := locks.Acquire(files.lockPath, 5*time.Second)
	if err != nil {
		return nil, err
	}
	l, err := yamlstore.Load(files.yamlPaths)
	if err != nil {
		_ = unlock()
		return nil, fmt.Errorf("load ledger: %w", err)
	}
	return &session{files: files, ledger: l, unlock: unlock}, nil
}

// openReadSession is the read-only counterpart (no lock — multiple readers OK).
func openReadSession(cmd *cobra.Command) (*session, error) {
	root, err := resolveDataDir(cmd)
	if err != nil {
		return nil, err
	}
	files, err := newDataDirFiles(root)
	if err != nil {
		return nil, err
	}
	if err := files.requireInitialized(); err != nil {
		return nil, err
	}
	l, err := yamlstore.Load(files.yamlPaths)
	if err != nil {
		return nil, fmt.Errorf("load ledger: %w", err)
	}
	return &session{files: files, ledger: l}, nil
}

// Save persists ledger changes (only dirty shards) and appends the given op
// to pending.yaml. The op is appended only after the ledger save succeeds —
// avoids enqueuing work the next sync would re-apply.
func (s *session) Save(op pending.Op) error {
	if _, err := yamlstore.Save(s.files.yamlPaths, s.ledger); err != nil {
		return fmt.Errorf("save ledger: %w", err)
	}
	if err := pending.Append(s.files.pendingPath, op); err != nil {
		return fmt.Errorf("append pending: %w", err)
	}
	return nil
}

// Close releases the lock if held. Safe to call on a read session.
func (s *session) Close() {
	if s.unlock != nil {
		_ = s.unlock()
	}
}
