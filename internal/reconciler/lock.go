package reconciler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrBusy reports that another sync already holds the round lock.
var ErrBusy = errors.New("another sync is already running")

// lockEnv overrides the runtime lock directory. It exists so tests can isolate
// their lock namespace; production leaves it unset.
const lockEnv = "STRMGEN_LOCK_DIR"

// Lock is an advisory cross-process lock held for a complete sync round.
type Lock struct{ file *os.File }

// AcquireLock takes the round lock in the default runtime directory.
func AcquireLock() (*Lock, error) {
	dir := os.Getenv(lockEnv)
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "strmgen-locks")
	}
	return AcquireLockIn(dir)
}

// AcquireLockIn takes the round lock in dir. This task conservatively
// serializes independent syncs, which is stronger than the required
// per-output-range exclusion and avoids stale lock state after a crash. The
// lock lives in a runtime directory outside every output root and the database,
// so acquiring it never creates product paths.
func AcquireLockIn(dir string) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("sync-%d.lock", os.Getuid()))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := flockExclusive(f); err != nil {
		f.Close()
		return nil, err
	}
	return &Lock{file: f}, nil
}

// Release drops the lock. It is safe on a nil receiver.
func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := flockUnlock(l.file)
	cerr := l.file.Close()
	if err != nil {
		return err
	}
	return cerr
}
