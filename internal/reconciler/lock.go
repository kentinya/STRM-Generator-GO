package reconciler

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"strm-generator-go/internal/securepath"
)

// ErrBusy reports that another sync already holds the process-wide round lock.
var ErrBusy = errors.New("another sync is already running")

// All supported CLI runs use this one fixed runtime namespace. It deliberately
// ignores TMPDIR, database paths, output roots, environment overrides and UID
// so two processes cannot split the coordination domain by changing config.
const runtimeLockDir = "/tmp/strmgen-locks"
const runtimeLockFile = "sync.lock"

// Lock is an advisory cross-process lock held for a complete sync round.
type Lock struct {
	file *os.File
	dir  *securepath.Dir
}

// AcquireLock is retained for package callers that need the shared lock. CLI
// code should use AcquireLockFor so conflicting product paths are rejected
// before the runtime lock directory is created.
func AcquireLock() (*Lock, error) {
	return AcquireLockFor(nil, "")
}

// AcquireLockFor takes the shared lock after proving its fixed runtime path is
// outside every configured output and database scope. This check precedes any
// mkdir or lock-file creation, including for dry-run.
func AcquireLockFor(outputs []string, databasePath string) (*Lock, error) {
	for _, output := range outputs {
		overlap, err := securepath.PathsOverlap(output, runtimeLockDir)
		if err != nil || overlap {
			return nil, errors.New("runtime lock path overlaps an output root")
		}
	}
	if databasePath != "" && pathsOverlap(databasePath, runtimeLockDir) {
		return nil, errors.New("runtime lock path overlaps database storage")
	}
	if databasePath != "" {
		for _, path := range []string{databasePath, databasePath + "-wal", databasePath + "-shm", databasePath + "-journal"} {
			inside, err := securepath.DirectoryContainsPath(runtimeLockDir, path)
			if err != nil || inside {
				return nil, errors.New("runtime lock path overlaps database storage")
			}
		}
	}
	if err := securepath.ValidateDirPath(runtimeLockDir); err != nil {
		return nil, errors.New("runtime lock directory has an unsafe path")
	}
	return acquireLockAt(runtimeLockDir)
}

// acquireLockAt is intentionally unexported and only used by package tests to
// isolate process-lock checks. The production command has no path override.
func acquireLockAt(dirPath string) (*Lock, error) {
	dir, err := securepath.OpenDir(dirPath, true, 0o700)
	if err != nil {
		return nil, errors.New("cannot prepare runtime lock directory")
	}
	info, err := dir.Stat()
	if err != nil {
		dir.Close()
		return nil, errors.New("cannot inspect runtime lock directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Geteuid()) || info.Mode().Perm()&0o022 != 0 {
		dir.Close()
		return nil, errors.New("runtime lock directory is not private to this user")
	}

	file, err := dir.OpenFile(runtimeLockFile, os.O_RDWR|os.O_CREATE|unix.O_NONBLOCK, 0o600)
	if err != nil {
		dir.Close()
		return nil, errors.New("cannot open runtime lock resource")
	}
	fileInfo, err := file.Stat()
	if err != nil || !fileInfo.Mode().IsRegular() {
		file.Close()
		dir.Close()
		return nil, errors.New("runtime lock resource is not a regular file")
	}
	fileStat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok || fileStat.Uid != uint32(os.Geteuid()) {
		file.Close()
		dir.Close()
		return nil, errors.New("runtime lock resource has an unexpected owner")
	}
	if err := flockExclusive(file); err != nil {
		file.Close()
		dir.Close()
		if errors.Is(err, ErrBusy) {
			return nil, ErrBusy
		}
		return nil, errors.New("cannot acquire runtime lock")
	}
	return &Lock{file: file, dir: dir}, nil
}

// Release drops the lock. It is safe on a nil receiver. The lock file is
// intentionally retained so another process never opens a replacement inode
// while a previous process still holds the original lock.
func (l *Lock) Release() error {
	if l == nil {
		return nil
	}
	var first error
	if l.file != nil {
		if err := flockUnlock(l.file); err != nil {
			first = err
		}
		if err := l.file.Close(); err != nil && first == nil {
			first = err
		}
		l.file = nil
	}
	if l.dir != nil {
		if err := l.dir.Close(); err != nil && first == nil {
			first = err
		}
		l.dir = nil
	}
	return first
}

func pathsOverlap(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	left, err := filepath.Abs(a)
	if err != nil {
		return true
	}
	right, err := filepath.Abs(b)
	if err != nil {
		return true
	}
	left, right = filepath.Clean(left), filepath.Clean(right)
	return within(left, right) || within(right, left)
}

func within(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
