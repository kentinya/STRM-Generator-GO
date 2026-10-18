package reconciler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"strm-generator-go/internal/openlist"
	"strm-generator-go/internal/securepath"
)

// F-14: each target must enumerate a prepared root with its own directory
// cursor. A residual for a later target in the same directory must still be
// detected even after an earlier target enumerated the same directory.
func TestRecoverDetectsResidualOnNonFirstTargetInPreparedRoot(t *testing.T) {
	root := t.TempDir()
	fs := &LocalFS{}
	if err := fs.PrepareRoots([]string{root}); err != nil {
		t.Fatalf("prepare roots: %v", err)
	}
	defer fs.CloseRoots()

	oldA := []byte("# strm-probe-version:1\nhttps://e.invalid/d/A.mkv\n")
	oldB := []byte("# strm-probe-version:1\nhttps://e.invalid/d/B.mkv\n")
	newA := []byte("# strm-probe-version:1\nhttps://e.invalid/d/A.mkv\n# strm-probe-size:1\n")
	newB := []byte("# strm-probe-version:1\nhttps://e.invalid/d/B.mkv\n# strm-probe-size:1\n")
	if err := fs.Publish(root, "A.strm", oldA); err != nil {
		t.Fatalf("publish A: %v", err)
	}
	if err := fs.Publish(root, "B.strm", oldB); err != nil {
		t.Fatalf("publish B: %v", err)
	}
	residual := newStagingName("B.strm", oldB, newB)
	if err := os.WriteFile(filepath.Join(root, residual), []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The first target has no residual and must recover cleanly. This consumes
	// the prepared root's directory cursor if enumeration is not independent.
	if err := fs.Recover(root, "A.strm", newA, true, false); err != nil {
		t.Fatalf("recover A: %v", err)
	}
	// The residual belongs to the second target; it must still be detected.
	if err := fs.Recover(root, "B.strm", newB, true, false); !errors.Is(err, ErrStagingPresent) {
		t.Fatalf("recover B = %v, want ErrStagingPresent", err)
	}
	// Repeated queries must keep detecting the same residual.
	if err := fs.Recover(root, "B.strm", newB, true, false); !errors.Is(err, ErrStagingPresent) {
		t.Fatalf("repeat recover B = %v, want ErrStagingPresent", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, residual)); string(raw) != "foreign" {
		t.Fatalf("residual was modified: %q", raw)
	}
}

// F-14/AC-9: enumeration must find a residual that appears after the first
// batch of directory entries, proving the reader streams in bounded batches
// instead of loading or losing the whole listing.
func TestFindStagingFilesStreamsPastLargeDirectory(t *testing.T) {
	root := t.TempDir()
	dir, err := securepath.OpenDir(root, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()

	base := "late.strm"
	residual := newStagingName(base, []byte("old"), []byte("new"))
	if err := os.WriteFile(filepath.Join(root, residual), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Fill more than one enumeration batch ahead of the residual.
	for i := 0; i < 300; i++ {
		name := filepath.Join(root, fmt.Sprintf("%04d.unrelated", i))
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	found, err := findStagingFiles(dir, base)
	if err != nil {
		t.Fatalf("find staging: %v", err)
	}
	if len(found) != 1 || found[0] != residual {
		t.Fatalf("late residual not found: %v", found)
	}
}

// AC-9: two residuals for one target are ambiguous. Enumeration retains at
// most two matches and Recover must refuse to guess, preserving both entries
// and the target.
func TestRecoverTwoResidualsIsConflictAndPreservesAll(t *testing.T) {
	root := t.TempDir()
	old, newer := writeUpdateFixture(t, root)
	first := newStagingName("A.strm", old, newer)
	second := newStagingName("A.strm", newer, old)
	if err := os.WriteFile(filepath.Join(root, first), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, second), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (LocalFS{}).Recover(root, "A.strm", newer, true, false); !errors.Is(err, ErrStagingPresent) {
		t.Fatalf("recover = %v, want ErrStagingPresent", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "A.strm")); string(raw) != string(old) {
		t.Fatalf("ambiguous recovery changed the target: %q", raw)
	}
	for name, want := range map[string]string{first: "one", second: "two"} {
		if raw, err := os.ReadFile(filepath.Join(root, name)); err != nil || string(raw) != want {
			t.Fatalf("residual %s not preserved: %q %v", name, raw, err)
		}
	}
}

// AC-9: enumeration failures must surface as errors and never be mistaken for
// an empty listing.
func TestFindStagingFilesReportsClosedDirectoryError(t *testing.T) {
	dir, err := securepath.OpenDir(t.TempDir(), false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := dir.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := findStagingFiles(dir, "A.strm"); err == nil {
		t.Fatal("closed directory must report an error, not an empty listing")
	}
}

// AC-9: a non-first root-level target in a prepared root, a subdirectory
// target, and an independent prepared root must each enumerate their own
// residual.
func TestRecoverDetectsResidualsAcrossPreparedTargetsAndSources(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	fs := &LocalFS{}
	if err := fs.PrepareRoots([]string{rootA, rootB}); err != nil {
		t.Fatalf("prepare roots: %v", err)
	}
	defer fs.CloseRoots()
	if err := fs.EnsureDir(rootA, "sub"); err != nil {
		t.Fatal(err)
	}

	old := []byte("old")
	newer := []byte("new")
	cases := []struct {
		root string
		rel  string
	}{
		{rootA, "A.strm"},
		{rootA, "B.strm"},
		{rootA, "sub/C.strm"},
		{rootB, "D.strm"},
	}
	for i, tc := range cases {
		if err := fs.Publish(tc.root, tc.rel, old); err != nil {
			t.Fatalf("publish %s: %v", tc.rel, err)
		}
		if i == len(cases)-1 {
			continue
		}
		if err := fs.Recover(tc.root, tc.rel, newer, true, false); err != nil {
			t.Fatalf("recover %s: %v", tc.rel, err)
		}
	}
	last := cases[len(cases)-1]
	residual := newStagingName(filepath.Base(last.rel), old, newer)
	if err := os.WriteFile(filepath.Join(last.root, residual), []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fs.Recover(last.root, last.rel, newer, true, false); !errors.Is(err, ErrStagingPresent) {
		t.Fatalf("recover %s = %v, want ErrStagingPresent", last.rel, err)
	}
	if err := fs.Recover(last.root, last.rel, newer, true, false); !errors.Is(err, ErrStagingPresent) {
		t.Fatalf("repeat recover %s = %v, want ErrStagingPresent", last.rel, err)
	}
}

// AC-6/AC-7: dry-run detection must not modify anything, and the following
// real run must still reconcile the residual.
func TestRecoverDryRunThenRealConverges(t *testing.T) {
	root := t.TempDir()
	old := []byte("old-owned")
	staged := []byte("staged-new")
	if err := (LocalFS{}).Publish(root, "A.strm", old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, newStagingName("A.strm", old, staged)), staged, 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotFiles(t, root)
	if err := (LocalFS{}).Recover(root, "A.strm", staged, true, true); !errors.Is(err, ErrStagingPresent) {
		t.Fatalf("dry-run recover = %v, want ErrStagingPresent", err)
	}
	if !mapsEqual(before, snapshotFiles(t, root)) {
		t.Fatal("dry-run recovery changed the filesystem")
	}
	if err := (LocalFS{}).Recover(root, "A.strm", staged, true, false); err != nil {
		t.Fatalf("real recover: %v", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "A.strm")); string(raw) != string(old) {
		t.Fatalf("owned target changed: %q", raw)
	}
	onlyEntryNamed(t, root, "A.strm")
}

// AC-6: an interruption after the exchange must converge even when the remote
// desired content changes before the restart. Recovery restores the previous
// owned content so the normal comparison redoes the update.
func TestRecoverConvergesAfterExchangeWhenDesiredChanges(t *testing.T) {
	root := t.TempDir()
	old := []byte("old-owned")
	published := []byte("published-new")
	later := []byte("later-remote")
	if err := (LocalFS{}).Publish(root, "A.strm", old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "A.strm"), published, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, newStagingName("A.strm", old, published)), old, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (LocalFS{}).Recover(root, "A.strm", later, true, false); err != nil {
		t.Fatalf("recover = %v, want nil", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "A.strm")); string(raw) != string(old) {
		t.Fatalf("displaced owned content not restored: %q", raw)
	}
	onlyEntryNamed(t, root, "A.strm")
}

// AC-6: an interruption before the exchange must converge even when the
// remote desired content changes before the restart.
func TestRecoverConvergesBeforeExchangeWhenDesiredChanges(t *testing.T) {
	root := t.TempDir()
	old := []byte("old-owned")
	staged := []byte("staged-new")
	later := []byte("later-remote")
	if err := (LocalFS{}).Publish(root, "A.strm", old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, newStagingName("A.strm", old, staged)), staged, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (LocalFS{}).Recover(root, "A.strm", later, true, false); err != nil {
		t.Fatalf("recover = %v, want nil", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "A.strm")); string(raw) != string(old) {
		t.Fatalf("owned target changed: %q", raw)
	}
	onlyEntryNamed(t, root, "A.strm")
}

// AC-6: when the target is gone, a residual provably holding this generator's
// own old content is stale and may be dropped so the creation path runs.
func TestRecoverDropsDisplacedOldWhenTargetAbsent(t *testing.T) {
	root := t.TempDir()
	old := []byte("old-owned")
	if err := (LocalFS{}).Publish(root, "A.strm", old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, newStagingName("A.strm", old, old)), old, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "A.strm")); err != nil {
		t.Fatal(err)
	}
	if err := (LocalFS{}).Recover(root, "A.strm", []byte("desired"), true, false); err != nil {
		t.Fatalf("recover = %v, want nil", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("residual not removed: %v", entries)
	}
}

// AC-6: an interruption after the exchange must converge to a changed remote
// desired URL, and the following identical run must be a stable no-op.
func TestInterruptedUpdateRecoveryConvergesWhenDesiredChanges(t *testing.T) {
	root := t.TempDir()
	store := newStore(t)
	lister := &fakeLister{
		dirs:    map[string][]openlist.Entry{"/Movies": {media("A.mkv", 1, shaA)}},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
	seed := Options{PublicPrefix: "https://seed.example.invalid"}
	if _, err := New(lister, store, seed).Run(context.Background(), []Source{sourceFor(root)}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	interrupted := Options{
		PublicPrefix: "https://first.example.invalid",
		FS: &LocalFS{afterExchange: func(*securepath.Dir, string) error {
			return errors.New("simulated interruption")
		}},
	}
	if _, err := New(lister, store, interrupted).Run(context.Background(), []Source{sourceFor(root)}); err == nil {
		t.Fatal("interruption must fail the run")
	}
	if !strings.Contains(mustRead(t, filepath.Join(root, "A.strm")), "first.example.invalid") {
		t.Fatalf("new bytes not durable before recovery")
	}

	recovery := Options{PublicPrefix: "https://second.example.invalid"}
	report, err := New(lister, store, recovery).Run(context.Background(), []Source{sourceFor(root)})
	if err != nil {
		t.Fatalf("recovery run: %v", err)
	}
	if report.Sources[0].Updated != 1 || report.Sources[0].Conflicts != 0 || report.Sources[0].Partial {
		t.Fatalf("recovery did not converge to the new desired URL: %+v", report.Sources[0])
	}
	if !strings.Contains(mustRead(t, filepath.Join(root, "A.strm")), "second.example.invalid") {
		t.Fatal("target not updated to the latest desired URL")
	}
	if entries, _ := os.ReadDir(root); len(entries) != 1 || entries[0].Name() != "A.strm" {
		t.Fatalf("recovery left staging residue: %v", entries)
	}

	beforeBytes := snapshotFiles(t, root)
	beforeTimes := statTimes(t, root)
	stable, err := New(lister, store, recovery).Run(context.Background(), []Source{sourceFor(root)})
	if err != nil {
		t.Fatalf("stable run: %v", err)
	}
	if stable.Sources[0].Skipped != 1 || stable.Sources[0].Updated != 0 {
		t.Fatalf("stable rerun did not skip: %+v", stable.Sources[0])
	}
	if !mapsEqual(beforeBytes, snapshotFiles(t, root)) {
		t.Fatal("stable rerun changed bytes")
	}
	afterTimes := statTimes(t, root)
	for name, mod := range beforeTimes {
		if !afterTimes[name].Equal(mod) {
			t.Fatalf("stable rerun changed mtime for %s", name)
		}
	}
}

// AC-6: an interruption before the exchange must converge to a changed remote
// desired URL.
func TestInterruptedBeforeExchangeRecoveryConvergesWhenDesiredChanges(t *testing.T) {
	root := t.TempDir()
	store := newStore(t)
	lister := &fakeLister{
		dirs:    map[string][]openlist.Entry{"/Movies": {media("A.mkv", 1, shaA)}},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
	seed := Options{PublicPrefix: "https://seed.example.invalid"}
	if _, err := New(lister, store, seed).Run(context.Background(), []Source{sourceFor(root)}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	interrupted := Options{
		PublicPrefix: "https://first.example.invalid",
		FS: &LocalFS{beforeExchange: func(*securepath.Dir, string) error {
			return errors.New("simulated pre-exchange interruption")
		}},
	}
	if _, err := New(lister, store, interrupted).Run(context.Background(), []Source{sourceFor(root)}); err == nil {
		t.Fatal("interruption must fail the run")
	}
	if !strings.Contains(mustRead(t, filepath.Join(root, "A.strm")), "seed.example.invalid") {
		t.Fatal("target changed before the interrupted exchange")
	}

	recovery := Options{PublicPrefix: "https://second.example.invalid"}
	report, err := New(lister, store, recovery).Run(context.Background(), []Source{sourceFor(root)})
	if err != nil {
		t.Fatalf("recovery run: %v", err)
	}
	if report.Sources[0].Updated != 1 || report.Sources[0].Conflicts != 0 || report.Sources[0].Partial {
		t.Fatalf("recovery did not converge to the new desired URL: %+v", report.Sources[0])
	}
	if !strings.Contains(mustRead(t, filepath.Join(root, "A.strm")), "second.example.invalid") {
		t.Fatal("target not updated to the latest desired URL")
	}
	if entries, _ := os.ReadDir(root); len(entries) != 1 || entries[0].Name() != "A.strm" {
		t.Fatalf("recovery left staging residue: %v", entries)
	}
}
