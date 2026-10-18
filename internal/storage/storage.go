// Package storage persists the minimal durable facts: file ownership, precise
// remote/local path mapping, the last complete successful reconciliation
// round, and the consecutive missing count. It never stores media fingerprints
// (those live in the STRM files).
//
// The write schema is upgraded transactionally and idempotently from the
// Slice 1 shape. Legacy rows keep their ownership and rounds, receive an
// unknown remote mapping and a missing_round marker of 0 so a historical count
// can never be used as deletion evidence. Round observations and pending
// missing paths live in a private in-memory scratch database bound to the
// Store; they are never written to the configured database and cannot leak
// into a later round.
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

// schemaVersion is the durable write schema this build understands.
const schemaVersion = 2

// maxInt64 is the SQLite integer ceiling. A missing count never overflows into
// a negative value or wraps around the confirmation threshold.
const maxInt64 = int64(9223372036854775807)

// scratchPageLimit caps the private in-memory round workspace (4 KiB pages).
// A pathological round fails explicitly instead of growing without bound.
const scratchPageLimit = 32768

const schema = `
CREATE TABLE IF NOT EXISTS ownership (
	source_key     TEXT    NOT NULL,
	relative_path  TEXT    NOT NULL,
	absolute_path  TEXT    NOT NULL,
	remote_path    TEXT    NOT NULL DEFAULT '',
	created_round  INTEGER NOT NULL,
	missing_count  INTEGER NOT NULL DEFAULT 0,
	missing_round  INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (source_key, relative_path)
);
CREATE TABLE IF NOT EXISTS rounds (
	source_key   TEXT    NOT NULL PRIMARY KEY,
	epoch        INTEGER NOT NULL,
	committed_at TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS ownership_missing_idx
	ON ownership (source_key, missing_count, missing_round);
`

// Ownership is one trustworthy owned STRM mapping. RemotePath is empty for a
// legacy row whose exact remote media path is not known; such a row is
// preserved but never eligible for cleanup.
type Ownership struct {
	SourceKey    string
	RelativePath string
	AbsolutePath string
	RemotePath   string
	MissingCount int64
	MissingRound int64
}

// Store wraps a SQLite database holding ownership and round facts.
type Store struct {
	db       *sql.DB
	readOnly bool
	dir      *securepath.Dir // pins the configured database parent for SQLite
	tempDir  *securepath.Dir // private WAL snapshot directory under the pinned DB parent
	tempName string
	tempBase *securepath.Dir
	legacy   bool // ownership lacks the precise-mapping columns (old read-only DB)
	scratch  *sql.DB
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
	if err := upgradeSchema(db); err != nil {
		db.Close()
		dir.Close()
		return nil, errors.New("initialize database schema")
	}
	scratch, err := openScratch()
	if err != nil {
		db.Close()
		dir.Close()
		return nil, errors.New("initialize round workspace")
	}
	return &Store{db: db, dir: dir, scratch: scratch}, nil
}

// OpenReadOnly opens an existing database without modifying it or creating
// journal/WAL/SHM files. For a WAL-mode database, which cannot be read
// read-only without shared-memory sidecars, it reads a private snapshot copy
// so the original file and its sidecars stay untouched. It returns
// ErrNoDatabase when the configured file is absent. It never migrates the
// schema: a Slice 1 database is readable with an unknown precise mapping.
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
	legacy, err := detectSchema(db)
	if err != nil {
		db.Close()
		dir.Close()
		return nil, errors.New("database schema is not safely readable")
	}
	scratch, err := openScratch()
	if err != nil {
		db.Close()
		dir.Close()
		return nil, errors.New("initialize round workspace")
	}
	return &Store{db: db, readOnly: true, dir: dir, legacy: legacy, scratch: scratch}, nil
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

// openScratch builds the private per-process round workspace. It is a memory
// database: dry-run and failed rounds leave no file behind and no marker can
// be consumed by a later process.
func openScratch() (*sql.DB, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
CREATE TABLE observed (
	source_key    TEXT NOT NULL,
	relative_path TEXT NOT NULL,
	PRIMARY KEY (source_key, relative_path)
);
CREATE TABLE missing (
	source_key    TEXT NOT NULL,
	relative_path TEXT NOT NULL,
	PRIMARY KEY (source_key, relative_path)
);
PRAGMA max_page_count = ` + fmt.Sprint(scratchPageLimit)); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// upgradeSchema validates and transactionally upgrades a writable database to
// the current schema. Slice 1 rows keep ownership, rounds and counts; the new
// columns default to an unknown remote mapping and a zero missing round so a
// legacy count can never qualify a deletion. A failed upgrade rolls back and
// leaves the previous database untouched.
func upgradeSchema(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var version int
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version > schemaVersion {
		return errors.New("database schema is newer than this build")
	}
	tables, err := tableNames(tx)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		if version != 0 {
			return errors.New("database schema version is unsupported")
		}
		if _, err := tx.Exec(schema); err != nil {
			return err
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
			return err
		}
		return tx.Commit()
	}
	for name := range tables {
		if name != "ownership" && name != "rounds" {
			return errors.New("database contains an unknown table")
		}
	}
	if !tables["ownership"] || !tables["rounds"] {
		return errors.New("database schema is incomplete")
	}
	columns, err := columnNames(tx, "ownership")
	if err != nil {
		return err
	}
	for _, required := range []string{"source_key", "relative_path", "absolute_path", "created_round", "missing_count"} {
		if !columns[required] {
			return errors.New("ownership table is missing a required column")
		}
	}
	roundColumns, err := columnNames(tx, "rounds")
	if err != nil {
		return err
	}
	for _, required := range []string{"source_key", "epoch", "committed_at"} {
		if !roundColumns[required] {
			return errors.New("rounds table is missing a required column")
		}
	}
	if !columns["remote_path"] {
		if _, err := tx.Exec(`ALTER TABLE ownership ADD COLUMN remote_path TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !columns["missing_round"] {
		if _, err := tx.Exec(`ALTER TABLE ownership ADD COLUMN missing_round INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS ownership_missing_idx
		ON ownership (source_key, missing_count, missing_round)`); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return err
	}
	return tx.Commit()
}

// detectSchema inspects a read-only database without modifying it and reports
// whether the precise-mapping columns are absent (a legacy Slice 1 database).
func detectSchema(db *sql.DB) (legacy bool, err error) {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return false, err
	}
	if version > schemaVersion {
		return false, errors.New("database schema is newer than this build")
	}
	tables, err := tableNames(db)
	if err != nil {
		return false, err
	}
	if len(tables) == 0 {
		return false, errors.New("database schema is missing")
	}
	for name := range tables {
		if name != "ownership" && name != "rounds" {
			return false, errors.New("database contains an unknown table")
		}
	}
	if !tables["ownership"] || !tables["rounds"] {
		return false, errors.New("database schema is incomplete")
	}
	columns, err := columnNames(db, "ownership")
	if err != nil {
		return false, err
	}
	for _, required := range []string{"source_key", "relative_path", "absolute_path", "created_round", "missing_count"} {
		if !columns[required] {
			return false, errors.New("ownership table is missing a required column")
		}
	}
	return !columns["remote_path"] || !columns["missing_round"], nil
}

type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func tableNames(q queryer) (map[string]bool, error) {
	rows, err := q.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tables := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables[name] = true
	}
	return tables, rows.Err()
}

func columnNames(q queryer, table string) (map[string]bool, error) {
	rows, err := q.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
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
	legacy, err := detectSchema(db)
	if err != nil {
		db.Close()
		cleanup()
		return nil, errors.New("database schema is not safely readable")
	}
	scratch, err := openScratch()
	if err != nil {
		db.Close()
		cleanup()
		return nil, errors.New("initialize round workspace")
	}
	return &Store{db: db, readOnly: true, tempDir: tempDir, tempName: tempName, tempBase: tempBase, legacy: legacy, scratch: scratch}, nil
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

// PreciseMappingAvailable reports whether the store carries the exact remote
// media path needed for a cleanup confirmation. A legacy read-only database
// reports false; rows are preserved but never eligible.
func (s *Store) PreciseMappingAvailable() bool { return s != nil && !s.legacy }

// Close releases the database handle, path pin, private scratch workspace and
// any private WAL snapshot.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	var first error
	if s.scratch != nil {
		if err := s.scratch.Close(); err != nil {
			first = err
		}
		s.scratch = nil
	}
	if s.db != nil {
		if err := s.db.Close(); err != nil && first == nil {
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
	o := Ownership{SourceKey: sourceKey, RelativePath: relativePath}
	var err error
	if s.legacy {
		err = s.db.QueryRow(
			`SELECT absolute_path FROM ownership WHERE source_key = ? AND relative_path = ?`,
			sourceKey, relativePath,
		).Scan(&o.AbsolutePath)
	} else {
		err = s.db.QueryRow(
			`SELECT absolute_path, remote_path, missing_count, missing_round
			 FROM ownership WHERE source_key = ? AND relative_path = ?`,
			sourceKey, relativePath,
		).Scan(&o.AbsolutePath, &o.RemotePath, &o.MissingCount, &o.MissingRound)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return Ownership{}, false, nil
	}
	if err != nil {
		return Ownership{}, false, err
	}
	return o, true, nil
}

// RecordOwnership registers a file that was successfully published or
// reconfirmed. It must only be called after the target is visible on disk. The
// mapping is refreshed and any previous missing evidence is cleared: an
// observed file is present, not missing.
func (s *Store) RecordOwnership(ctx context.Context, o Ownership, epoch int64) error {
	if s.readOnly {
		return errors.New("store is read-only")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ownership (source_key, relative_path, absolute_path, remote_path, created_round, missing_count, missing_round)
		 VALUES (?, ?, ?, ?, ?, 0, 0)
		 ON CONFLICT(source_key, relative_path) DO UPDATE SET
			absolute_path = excluded.absolute_path,
			remote_path = CASE WHEN excluded.remote_path = '' THEN ownership.remote_path ELSE excluded.remote_path END,
			missing_count = 0,
			missing_round = 0`,
		o.SourceKey, o.RelativePath, o.AbsolutePath, o.RemotePath, epoch,
	)
	return err
}

// ClearMissing conservatively clears any committed missing evidence for one
// target. It is safe for a row that does not exist and is called as soon as a
// source file is observed, even if later processing fails.
func (s *Store) ClearMissing(ctx context.Context, sourceKey, relativePath string) error {
	if s.readOnly {
		return errors.New("store is read-only")
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE ownership SET missing_count = 0, missing_round = 0
		 WHERE source_key = ? AND relative_path = ?`,
		sourceKey, relativePath,
	)
	return err
}

// RemoveOwnership drops one ownership row after its target was safely removed.
func (s *Store) RemoveOwnership(ctx context.Context, sourceKey, relativePath string) error {
	if s.readOnly {
		return errors.New("store is read-only")
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM ownership WHERE source_key = ? AND relative_path = ?`,
		sourceKey, relativePath,
	)
	return err
}

// ListOwnershipAfter returns one bounded batch of ownership rows for a source,
// ordered by relative path. Keyset pagination never uses OFFSET, so rows
// removed during iteration cannot shift later batches or be skipped.
func (s *Store) ListOwnershipAfter(ctx context.Context, sourceKey, after string, limit int) ([]Ownership, error) {
	if limit <= 0 {
		limit = 200
	}
	if s.legacy {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT relative_path, absolute_path, remote_path, missing_count, missing_round
		 FROM ownership WHERE source_key = ? AND relative_path > ?
		 ORDER BY relative_path LIMIT ?`,
		sourceKey, after, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Ownership
	for rows.Next() {
		o := Ownership{SourceKey: sourceKey}
		if err := rows.Scan(&o.RelativePath, &o.AbsolutePath, &o.RemotePath, &o.MissingCount, &o.MissingRound); err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, rows.Err()
}

// ListEligibleAfter returns one bounded batch of ownership rows whose committed
// evidence already reaches the threshold. Only rows with a proven missing
// round and an exact remote mapping qualify.
func (s *Store) ListEligibleAfter(ctx context.Context, sourceKey, after string, threshold int, limit int) ([]Ownership, error) {
	if limit <= 0 {
		limit = 200
	}
	if s.legacy {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT relative_path, absolute_path, remote_path, missing_count, missing_round
		 FROM ownership
		 WHERE source_key = ? AND relative_path > ? AND missing_count >= ? AND missing_round > 0 AND remote_path != ''
		 ORDER BY relative_path LIMIT ?`,
		sourceKey, after, threshold, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Ownership
	for rows.Next() {
		o := Ownership{SourceKey: sourceKey}
		if err := rows.Scan(&o.RelativePath, &o.AbsolutePath, &o.RemotePath, &o.MissingCount, &o.MissingRound); err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, rows.Err()
}

// CountUnmappedMissing reports how many rows hold a committed missing count but
// lack the precise remote mapping needed for a confirmation. They are reported
// and preserved, never guessed or deleted.
func (s *Store) CountUnmappedMissing(ctx context.Context, sourceKey string) (int64, error) {
	if s.legacy {
		return 0, nil
	}
	var count int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ownership
		 WHERE source_key = ? AND missing_count > 0 AND (remote_path = '' OR missing_round = 0)`,
		sourceKey,
	).Scan(&count)
	return count, err
}

// ResetRound discards every observation and pending missing path for one
// source before a new round starts, so a failed or interrupted round can never
// feed the next one.
func (s *Store) ResetRound(ctx context.Context, sourceKey string) error {
	if s.scratch == nil {
		return nil
	}
	if _, err := s.scratch.ExecContext(ctx, `DELETE FROM observed WHERE source_key = ?`, sourceKey); err != nil {
		return err
	}
	if _, err := s.scratch.ExecContext(ctx, `DELETE FROM missing WHERE source_key = ?`, sourceKey); err != nil {
		return err
	}
	return nil
}

// MarkObserved records that one exact target was observed in the current round
// (a listing hit, skip, update or a successful Get). The marker lives only in
// the private scratch workspace.
func (s *Store) MarkObserved(ctx context.Context, sourceKey, relativePath string) error {
	if s.scratch == nil {
		return nil
	}
	_, err := s.scratch.ExecContext(ctx,
		`INSERT OR IGNORE INTO observed (source_key, relative_path) VALUES (?, ?)`,
		sourceKey, relativePath)
	return err
}

// IsObserved reports whether the current round already observed one target.
func (s *Store) IsObserved(ctx context.Context, sourceKey, relativePath string) (bool, error) {
	if s.scratch == nil {
		return false, nil
	}
	var one int
	err := s.scratch.QueryRowContext(ctx,
		`SELECT 1 FROM observed WHERE source_key = ? AND relative_path = ?`,
		sourceKey, relativePath).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// AddPendingMissing stages one bounded batch of missing observations for the
// current round. Nothing reaches the ownership counts until CommitRound.
func (s *Store) AddPendingMissing(ctx context.Context, sourceKey string, relativePaths []string) error {
	if s.scratch == nil || len(relativePaths) == 0 {
		return nil
	}
	tx, err := s.scratch.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO missing (source_key, relative_path) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, rel := range relativePaths {
		if _, err := statement.ExecContext(ctx, sourceKey, rel); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PendingMissing reports the number of missing observations staged for a
// source in the current round.
func (s *Store) PendingMissing(ctx context.Context, sourceKey string) (int64, error) {
	if s.scratch == nil {
		return 0, nil
	}
	var count int64
	err := s.scratch.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM missing WHERE source_key = ?`, sourceKey).Scan(&count)
	return count, err
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

// CommitRound records a complete successful reconciliation round and applies
// the staged missing observations in the same transaction. Every target gains
// at most one count per distinct complete epoch; a repeated commit of the same
// epoch is a no-op and an older epoch can never overwrite a newer round.
func (s *Store) CommitRound(ctx context.Context, sourceKey string, epoch int64, at time.Time) error {
	if s.readOnly {
		return errors.New("store is read-only")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	after := ""
	for {
		paths, err := s.pendingBatch(ctx, sourceKey, after, 200)
		if err != nil {
			return err
		}
		if len(paths) == 0 {
			break
		}
		for _, rel := range paths {
			if _, err := tx.ExecContext(ctx,
				`UPDATE ownership
				 SET missing_count = CASE WHEN missing_count < ? THEN missing_count + 1 ELSE ? END,
				     missing_round = ?
				 WHERE source_key = ? AND relative_path = ? AND missing_round < ?`,
				maxInt64, maxInt64, epoch, sourceKey, rel, epoch,
			); err != nil {
				return err
			}
		}
		after = paths[len(paths)-1]
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO rounds (source_key, epoch, committed_at) VALUES (?, ?, ?)
		 ON CONFLICT(source_key) DO UPDATE SET
			epoch = excluded.epoch,
			committed_at = excluded.committed_at
		 WHERE excluded.epoch > rounds.epoch`,
		sourceKey, epoch, at.UTC().Format(time.RFC3339),
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if s.scratch != nil {
		if _, err := s.scratch.ExecContext(ctx, `DELETE FROM missing WHERE source_key = ?`, sourceKey); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) pendingBatch(ctx context.Context, sourceKey, after string, limit int) ([]string, error) {
	if s.scratch == nil {
		return nil, nil
	}
	rows, err := s.scratch.QueryContext(ctx,
		`SELECT relative_path FROM missing WHERE source_key = ? AND relative_path > ?
		 ORDER BY relative_path LIMIT ?`,
		sourceKey, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var rel string
		if err := rows.Scan(&rel); err != nil {
			return nil, err
		}
		paths = append(paths, rel)
	}
	return paths, rows.Err()
}
