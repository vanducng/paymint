// Package locks wraps gofrs/flock for the two paymint use cases:
// the data-dir lock (.paymint/lock, held during any write) and the OAuth
// token lock (Phase 4 will reuse this package). Callers always defer the
// returned unlocker; never hold a lock across an interactive prompt.
package locks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// Acquire takes an exclusive lock on path, creating the file (mode 0600) if
// missing. timeout caps the wait; pass 0 for non-blocking. Returns an
// unlocker; the caller must defer it.
func Acquire(path string, timeout time.Duration) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("lock dir: %w", err)
	}
	lk := flock.New(path)
	if timeout == 0 {
		ok, err := lk.TryLock()
		if err != nil {
			return nil, fmt.Errorf("trylock %s: %w", path, err)
		}
		if !ok {
			return nil, fmt.Errorf("lock %s: held by another process", path)
		}
		return lk.Unlock, nil
	}
	deadline := time.Now().Add(timeout)
	for {
		ok, err := lk.TryLock()
		if err != nil {
			return nil, fmt.Errorf("trylock %s: %w", path, err)
		}
		if ok {
			return lk.Unlock, nil
		}
		if time.Now().After(deadline) {
			return nil, errors.New("lock timeout: another paymint process is busy")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
