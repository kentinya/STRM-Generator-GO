package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
