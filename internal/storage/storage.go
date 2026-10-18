// Package storage persists the minimal durable facts: file ownership, path
// mapping, and the last complete successful reconciliation round. It never
// stores media fingerprints (those live in the STRM files) and never advances
// missing-count evidence in this task.
package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"strm-generator-go/internal/securepath"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS ownership (
	source_key     TEXT    NOT NULL,
	relative_path  TEXT    NOT NULL,
	absolute_path  TEXT    NOT NULL,
	created_round  INTEGER NOT NULL,
	missing_count  INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (source_key, relative_path)
);
CREATE TABLE IF NOT EXISTS rounds (
	source_key   TEXT    NOT NULL PRIMARY KEY,
	epoch        INTEGER NOT NULL,
	committed_at TEXT    NOT NULL
);
`

// Ownership is one trustworthy owned STRM mapping.
type Ownership struct {
	SourceKey    string
	RelativePath string
	AbsolutePath string
}

// Store wraps a SQLite database holding ownership and round facts.
type Store struct {
	db       *sql.DB
	readOnly bool
	dir      *securepath.Dir // pins the configured database parent for SQLite
	tempDir  *securepath.Dir // private WAL snapshot directory under the pinned DB parent
	tempName string
	tempBase *securepath.Dir
}

// Open opens or creates the ownership database for writing. Parent traversal,
// final-file inspection, and file creation are descriptor-relative and reject
// symlinks. SQLite receives an escaped URI rooted at the pinned parent fd.
func Open(path string) (*Store, error) {
	dir, base, err := openDatabaseParent(path, true)
	if err != nil {
		return nil, fmt.Errorf("prepare database: %w", err)
	}
	info, err := dir.Lstat(base)
	if errors.Is(err, os.ErrNotExist) {
		file, createErr := dir.OpenFile(base, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if createErr != nil {
			dir.Close()
			return nil, errors.New("create database file")
		}
		if closeErr := file.Close(); closeErr != nil {
			dir.Close()
			return nil, errors.New("close new database file")
		}
	} else if err != nil {
		dir.Close()
		return nil, errors.New("inspect database file")
	} else if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		dir.Close()
		return nil, errors.New("database path is not a regular non-symlink file")
	}
	dsn, err := sqliteDSN(dir, base, "_pragma=busy_timeout(5000)")
	if err != nil {
		dir.Close()
		return nil, errors.New("construct database address")
	}
	db, err := openSQL(dsn)
	if err != nil {
		dir.Close()
		return nil, errors.New("open database")
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		dir.Close()
		return nil, errors.New("initialize database schema")
	}
	return &Store{db: db, dir: dir}, nil
}

// OpenReadOnly opens an existing database without modifying it or creating
// journal/WAL/SHM files. For a WAL-mode database, which cannot be read
// read-only without shared-memory sidecars, it reads a private snapshot copy
// so the original file and its sidecars stay untouched. It returns
// ErrNoDatabase when the configured file is absent.
func OpenReadOnly(path string, forbiddenOutputRoots ...string) (*Store, error) {
	dir, base, err := openDatabaseParent(path, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoDatabase
	}
	if err != nil {
		return nil, err
	}
	info, err := dir.Lstat(base)
	if errors.Is(err, os.ErrNotExist) {
		dir.Close()
		return nil, ErrNoDatabase
	}
	if err != nil {
		dir.Close()
		return nil, errors.New("inspect database file")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		dir.Close()
		return nil, errors.New("database path is not a regular non-symlink file")
	}
	wal, err := isWALMode(dir, base)
	if err != nil {
		dir.Close()
		return nil, errors.New("database is not safely readable")
	}
	if wal {
		store, err := openWALSnapshot(dir, base, filepath.Dir(filepath.Clean(path)), forbiddenOutputRoots)
		dir.Close()
		return store, err
	}
	dsn, err := sqliteDSN(dir, base, "mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		dir.Close()
		return nil, errors.New("construct database address")
	}
	db, err := openSQL(dsn)
	if err != nil {
		dir.Close()
		return nil, errors.New("database is not safely readable")
	}
	return &Store{db: db, readOnly: true, dir: dir}, nil
}

func openDatabaseParent(path string, create bool) (*securepath.Dir, string, error) {
	if path == "" || strings.ContainsRune(path, 0) {
		return nil, "", errors.New("invalid database path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", errors.New("cannot resolve database path")
	}
	abs = filepath.Clean(abs)
	base := filepath.Base(abs)
	if base == "." || base == string(filepath.Separator) || base == ".." {
		return nil, "", errors.New("invalid database file name")
	}
	dir, err := securepath.OpenDir(filepath.Dir(abs), create, 0o755)
	if err != nil {
		return nil, "", err
	}
	return dir, base, nil
}

func sqliteDSN(dir *securepath.Dir, name, query string) (string, error) {
	path, err := dir.ProcPath(name)
	if err != nil {
		return "", err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	return u.String() + "?" + query, nil
}

func openSQL(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// isWALMode reads the SQLite header from an opened no-follow file descriptor.
// Read/write version bytes at offsets 18/19 equal 2 for WAL mode.
func isWALMode(dir *securepath.Dir, name string) (bool, error) {
	f, err := dir.OpenFile(name, os.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return false, errors.New("database is not a regular file")
	}
	header := make([]byte, 20)
	if _, err := io.ReadFull(f, header); err != nil {
		return false, errors.New("cannot read database header")
	}
	if string(header[:16]) != "SQLite format 3\x00" {
		return false, errors.New("not a SQLite database")
	}
	return header[18] == 2 && header[19] == 2, nil
}

// openWALSnapshot copies a database and optional sidecars into a private
// directory under the already validated database parent, then opens that copy
// read-only. It deliberately ignores TMPDIR so a configured output root can
// never receive dry-run snapshot files through an environment override.
func openWALSnapshot(sourceDir *securepath.Dir, base, parentPath string, forbiddenOutputRoots []string) (*Store, error) {
	tempBase, err := sourceDir.Duplicate()
	if err != nil {
		return nil, errors.New("pin read-only snapshot parent")
	}
	tempName, tempDir, err := createSnapshotDirectory(tempBase, parentPath, forbiddenOutputRoots)
	if err != nil {
		_ = tempBase.Close()
		return nil, errors.New("create read-only snapshot directory")
	}
	cleanup := func() {
		_ = removeSnapshot(tempBase, tempDir, tempName)
	}
	copyOne := func(srcName, dstName string, required bool) error {
		in, err := sourceDir.OpenFile(srcName, os.O_RDONLY|unix.O_NONBLOCK, 0)
		if errors.Is(err, os.ErrNotExist) && !required {
			return nil
		}
		if err != nil {
			return err
		}
		defer in.Close()
		info, err := in.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("snapshot source is not a regular file")
		}
		out, err := tempDir.OpenFile(dstName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		syncErr := out.Sync()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if syncErr != nil {
			return syncErr
		}
		return closeErr
	}
	if err := copyOne(base, "snapshot.db", true); err != nil {
		cleanup()
		return nil, errors.New("snapshot database")
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := copyOne(base+suffix, "snapshot.db"+suffix, false); err != nil {
			cleanup()
			return nil, errors.New("snapshot database sidecar")
		}
	}
	dsn, err := sqliteDSN(tempDir, "snapshot.db", "mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		cleanup()
		return nil, errors.New("construct database snapshot address")
	}
	db, err := openSQL(dsn)
	if err != nil {
		cleanup()
		return nil, errors.New("open database snapshot")
	}
	return &Store{db: db, readOnly: true, tempDir: tempDir, tempName: tempName, tempBase: tempBase}, nil
}

func createSnapshotDirectory(parent *securepath.Dir, parentPath string, forbiddenOutputRoots []string) (string, *securepath.Dir, error) {
	for attempt := 0; attempt < 10; attempt++ {
		var token [12]byte
		if _, err := rand.Read(token[:]); err != nil {
			return "", nil, err
		}
		name := ".strmgen-ro-" + hex.EncodeToString(token[:])
		candidate := filepath.Join(parentPath, name)
		unsafe := false
		for _, output := range forbiddenOutputRoots {
			overlap, err := securepath.PathsOverlap(candidate, output)
			if err != nil || overlap {
				unsafe = true
				break
			}
		}
		if unsafe {
			continue
		}
		if err := parent.Mkdir(name, 0o700); err != nil {
			if errors.Is(err, unix.EEXIST) {
				continue
			}
			return "", nil, err
		}
		dir, err := parent.OpenSubdir(name, false, 0)
		if err != nil {
			_ = parent.RemoveDir(name)
			return "", nil, err
		}
		return name, dir, nil
	}
	return "", nil, errors.New("cannot allocate snapshot directory")
}

func removeSnapshot(parent, dir *securepath.Dir, name string) error {
	var first error
	if dir != nil {
		for _, file := range []string{"snapshot.db-wal", "snapshot.db-shm", "snapshot.db-journal", "snapshot.db"} {
			if err := dir.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) && first == nil {
				first = err
			}
		}
		if err := dir.Close(); err != nil && first == nil {
			first = err
		}
	}
	if parent != nil {
		if name != "" {
			if err := parent.RemoveDir(name); err != nil && !errors.Is(err, os.ErrNotExist) && first == nil {
				first = err
			}
		}
		if err := parent.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// ErrNoDatabase signals an absent database in read-only mode.
var ErrNoDatabase = errors.New("database does not exist")

// ReadOnly reports whether the store refuses writes.
func (s *Store) ReadOnly() bool { return s.readOnly }

// Close releases the database handle, path pin and any private WAL snapshot.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	var first error
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			first = err
		}
		s.db = nil
	}
	if s.dir != nil {
		if err := s.dir.Close(); err != nil && first == nil {
			first = err
		}
		s.dir = nil
	}
	if s.tempDir != nil || s.tempBase != nil {
		if err := removeSnapshot(s.tempBase, s.tempDir, s.tempName); err != nil && first == nil {
			first = err
		}
		s.tempDir, s.tempBase, s.tempName = nil, nil, ""
	}
	return first
}

// Lookup returns the ownership record for a relative path when present.
func (s *Store) Lookup(sourceKey, relativePath string) (Ownership, bool, error) {
	var o Ownership
	o.SourceKey = sourceKey
	o.RelativePath = relativePath
	err := s.db.QueryRow(
		`SELECT absolute_path FROM ownership WHERE source_key = ? AND relative_path = ?`,
		sourceKey, relativePath,
	).Scan(&o.AbsolutePath)
	if errors.Is(err, sql.ErrNoRows) {
		return Ownership{}, false, nil
	}
	if err != nil {
		return Ownership{}, false, err
	}
	return o, true, nil
}

// RecordOwnership registers a file that was successfully published. It must
// only be called after the target is visible on disk.
func (s *Store) RecordOwnership(ctx context.Context, o Ownership, epoch int64) error {
	if s.readOnly {
		return errors.New("store is read-only")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ownership (source_key, relative_path, absolute_path, created_round, missing_count)
		 VALUES (?, ?, ?, ?, 0)
		 ON CONFLICT(source_key, relative_path) DO UPDATE SET absolute_path = excluded.absolute_path`,
		o.SourceKey, o.RelativePath, o.AbsolutePath, epoch,
	)
	return err
}

// NextEpoch returns the next round number for a source.
func (s *Store) NextEpoch(sourceKey string) (int64, error) {
	var epoch sql.NullInt64
	err := s.db.QueryRow(`SELECT epoch FROM rounds WHERE source_key = ?`, sourceKey).Scan(&epoch)
	if errors.Is(err, sql.ErrNoRows) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	return epoch.Int64 + 1, nil
}

// CommitRound records a complete successful reconciliation round.
func (s *Store) CommitRound(ctx context.Context, sourceKey string, epoch int64, at time.Time) error {
	if s.readOnly {
		return errors.New("store is read-only")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO rounds (source_key, epoch, committed_at) VALUES (?, ?, ?)
		 ON CONFLICT(source_key) DO UPDATE SET epoch = excluded.epoch, committed_at = excluded.committed_at`,
		sourceKey, epoch, at.UTC().Format(time.RFC3339),
	)
	return err
}
