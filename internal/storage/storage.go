// Package storage persists the minimal durable facts: file ownership, path
// mapping, and the last complete successful reconciliation round. It never
// stores media fingerprints (those live in the STRM files) and never advances
// missing-count evidence in this task.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

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
	tempDir  string // removed on Close for a WAL snapshot copy
}

// Open opens or creates the ownership database for writing.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize schema: %w", err)
	}
	return &Store{db: db}, nil
}

// OpenReadOnly opens an existing database without modifying it or creating
// journal/WAL/SHM files. For a WAL-mode database, which cannot be read
// read-only without shared-memory sidecars, it reads a private snapshot copy so
// the original file and its sidecars stay untouched. It returns ErrNoDatabase
// when the file is absent so the caller can treat every path as unowned without
// creating anything.
func OpenReadOnly(path string) (*Store, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoDatabase
		}
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("database path is a directory")
	}
	wal, err := isWALMode(path)
	if err != nil {
		return nil, fmt.Errorf("database is not safely readable: %w", err)
	}
	if wal {
		return openWALSnapshot(path)
	}
	dsn := "file:" + path + "?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database read-only: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("database is not safely readable: %w", err)
	}
	return &Store{db: db, readOnly: true}, nil
}

// isWALMode reads the SQLite header to detect WAL mode (format read/write
// version bytes at offset 18/19 equal 2). This never opens the database, so it
// cannot create sidecars.
func isWALMode(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	header := make([]byte, 20)
	if _, err := io.ReadFull(f, header); err != nil {
		return false, fmt.Errorf("cannot read database header")
	}
	if string(header[:16]) != "SQLite format 3\x00" {
		return false, fmt.Errorf("not a SQLite database")
	}
	return header[18] == 2 && header[19] == 2, nil
}

// openWALSnapshot copies the database and any WAL/SHM sidecars into a private
// temporary directory and opens the copy read-only. The copy is removed on
// Close. The original database and its sidecars are never modified.
func openWALSnapshot(path string) (*Store, error) {
	tempDir, err := os.MkdirTemp("", "strmgen-ro-*")
	if err != nil {
		return nil, fmt.Errorf("create read-only snapshot directory: %w", err)
	}
	copyFile := func(src, dst string) error {
		in, err := os.Open(src)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	}
	target := filepath.Join(tempDir, "snapshot.db")
	if err := copyFile(path, target); err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("snapshot database: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := copyFile(path+suffix, target+suffix); err != nil {
			os.RemoveAll(tempDir)
			return nil, fmt.Errorf("snapshot %s: %w", suffix, err)
		}
	}
	dsn := "file:" + target + "?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("open database snapshot: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("database snapshot is not safely readable: %w", err)
	}
	return &Store{db: db, readOnly: true, tempDir: tempDir}, nil
}

// ErrNoDatabase signals an absent database in read-only mode.
var ErrNoDatabase = errors.New("database does not exist")

// ReadOnly reports whether the store refuses writes.
func (s *Store) ReadOnly() bool { return s.readOnly }

// Close releases the database handle and removes any snapshot copy.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	err := s.db.Close()
	if s.tempDir != "" {
		os.RemoveAll(s.tempDir)
	}
	return err
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
