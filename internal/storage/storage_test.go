package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecordLookupAndRound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "strmgen.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	if _, ok, err := s.Lookup("/Movies", "A.strm"); err != nil || ok {
		t.Fatalf("unexpected lookup result: %v %v", ok, err)
	}
	epoch, err := s.NextEpoch("/Movies")
	if err != nil || epoch != 1 {
		t.Fatalf("first epoch = %d, %v", epoch, err)
	}
	if err := s.RecordOwnership(ctx, Ownership{SourceKey: "/Movies", RelativePath: "A.strm", AbsolutePath: "/out/A.strm"}, epoch); err != nil {
		t.Fatalf("record: %v", err)
	}
	o, ok, err := s.Lookup("/Movies", "A.strm")
	if err != nil || !ok || o.AbsolutePath != "/out/A.strm" {
		t.Fatalf("lookup: %+v %v %v", o, ok, err)
	}
	if err := s.CommitRound(ctx, "/Movies", epoch, time.Unix(0, 0)); err != nil {
		t.Fatalf("commit: %v", err)
	}
	next, err := s.NextEpoch("/Movies")
	if err != nil || next != 2 {
		t.Fatalf("next epoch = %d, %v", next, err)
	}
}

func TestOpenReadOnlyDoesNotCreate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "strmgen.db")
	_, err := OpenReadOnly(path)
	if !errors.Is(err, ErrNoDatabase) {
		t.Fatalf("expected ErrNoDatabase, got %v", err)
	}
}

func TestOpenReadOnlyWALDoesNotCreateSidecars(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strmgen.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.RecordOwnership(ctx, Ownership{SourceKey: "/Movies", RelativePath: "A.strm", AbsolutePath: "/out/A.strm"}, 1); err != nil {
		t.Fatal(err)
	}
	s.Close()
	// Convert to WAL mode and checkpoint so no sidecars remain.
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s2.db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	s2.Close()

	before := sidecarSet(path)
	if len(before) != 0 {
		t.Fatalf("unexpected sidecars before read: %v", before)
	}
	ro, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("read-only open: %v", err)
	}
	defer ro.Close()
	if _, ok, err := ro.Lookup("/Movies", "A.strm"); err != nil || !ok {
		t.Fatalf("lookup on snapshot: %v %v", ok, err)
	}
	if after := sidecarSet(path); len(after) != 0 {
		t.Fatalf("read-only WAL open created sidecars: %v", after)
	}
}

func TestWALSnapshotIgnoresTMPDIRAndCleansItsPinnedParent(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "strmgen.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RecordOwnership(context.Background(), Ownership{
		SourceKey: "source", RelativePath: "A.strm", AbsolutePath: "/out/A.strm",
	}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// Remove empty sidecars after checkpoint; the database header remains WAL.
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}

	output := filepath.Join(base, "out")
	if err := os.Mkdir(output, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", output)
	ro, err := OpenReadOnly(path, output)
	if err != nil {
		t.Fatalf("read-only WAL snapshot: %v", err)
	}
	if _, ok, err := ro.Lookup("source", "A.strm"); err != nil || !ok {
		t.Fatalf("snapshot lookup: ok=%t err=%v", ok, err)
	}
	if entries, err := os.ReadDir(output); err != nil || len(entries) != 0 {
		t.Fatalf("TMPDIR/output received snapshot files: entries=%v err=%v", entries, err)
	}
	if err := ro.Close(); err != nil {
		t.Fatalf("close snapshot: %v", err)
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".strmgen-ro-") {
			t.Fatalf("snapshot directory was not removed: %s", entry.Name())
		}
	}
}

func sidecarSet(path string) []string {
	var found []string
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Stat(path + suffix); err == nil {
			found = append(found, filepath.Base(path+suffix))
		}
	}
	return found
}

func TestReadOnlyRejectsWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strmgen.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	ro, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("read-only open: %v", err)
	}
	defer ro.Close()
	if err := ro.RecordOwnership(context.Background(), Ownership{SourceKey: "/Movies", RelativePath: "A.strm", AbsolutePath: "/out/A.strm"}, 1); err == nil {
		t.Fatal("read-only store must refuse writes")
	}
	if err := ro.CommitRound(context.Background(), "/Movies", 1, time.Now()); err == nil {
		t.Fatal("read-only store must refuse round commits")
	}
}

func TestSQLitePathCharactersAreNotInterpretedAsURIOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db ?name#percent%.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open exact database path: %v", err)
	}
	if err := s.RecordOwnership(context.Background(), Ownership{
		SourceKey: "source", RelativePath: "A.strm", AbsolutePath: "/out/A.strm",
	}, 1); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("configured database path was not created: %v", err)
	}
	if matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), "db *")); err != nil {
		t.Fatal(err)
	} else if len(matches) != 1 || matches[0] != path {
		t.Fatalf("unexpected database files: %v", matches)
	}
	ro, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("read-only open: %v", err)
	}
	defer ro.Close()
	if _, ok, err := ro.Lookup("source", "A.strm"); err != nil || !ok {
		t.Fatalf("configured database did not retain ownership: ok=%v err=%v", ok, err)
	}
}

func TestOpenRejectsDatabaseSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.db")
	if err := os.WriteFile(target, []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.db")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(link); err == nil {
		t.Fatal("database symlink must be rejected")
	}
	if raw, err := os.ReadFile(target); err != nil || string(raw) != "not a database" {
		t.Fatalf("symlink target was changed: %q %v", raw, err)
	}
}
