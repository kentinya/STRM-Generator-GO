package reconciler

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"strm-generator-go/internal/openlist"
	"strm-generator-go/internal/securepath"
	"strm-generator-go/internal/storage"
)

func cleanupOptions() Options {
	return Options{
		PublicPrefix:         "https://openlist.example.invalid",
		CleanupEnabled:       true,
		MissingConfirmations: 3,
	}
}

func removeListing(lister *fakeLister, dirPath, name string) {
	entries := lister.dirs[dirPath]
	filtered := make([]openlist.Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.Name != name {
			filtered = append(filtered, entry)
		}
	}
	lister.dirs[dirPath] = filtered
}

func runSource(t *testing.T, lister *fakeLister, store *storage.Store, root string, opts Options) *Report {
	t.Helper()
	report, err := New(lister, store, opts).Run(context.Background(), []Source{sourceFor(root)})
	if report == nil {
		t.Fatalf("run returned no report: %v", err)
	}
	return report
}

// seedStandard creates the complete initial tree and asserts four real files.
func seedStandard(t *testing.T, lister *fakeLister, store *storage.Store, root string) {
	t.Helper()
	report := runSource(t, lister, store, root, cleanupOptions())
	if report.Sources[0].Created != 4 || report.Sources[0].Partial {
		t.Fatalf("seed result: %+v", report.Sources[0])
	}
}

// removeRemote makes one remote file unobservable: it disappears from the
// listing and the exact target Get fails.
func removeRemote(lister *fakeLister, name string) {
	removeListing(lister, "/Movies", name)
	lister.getErr["/Movies/"+name] = errors.New("target not read")
}

func countFor(t *testing.T, store *storage.Store, rel string) int64 {
	t.Helper()
	o, ok, err := store.Lookup("/Movies", rel)
	if err != nil || !ok {
		t.Fatalf("lookup %s: ok=%v err=%v", rel, ok, err)
	}
	return o.MissingCount
}

// newStoreWithPath returns a writable store and the raw database path so a
// test can seed a legacy row without adding production accessors.
func newStoreWithPath(t *testing.T) (*storage.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data", "strmgen.db")
	s, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func rawExec(t *testing.T, path, query string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupRetainsUntilCommittedThresholdThenDeletes(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	seedStandard(t, lister, store, root)
	removeRemote(lister, "Sample.mkv")
	for round := 1; round <= 3; round++ {
		report := runSource(t, lister, store, root, cleanupOptions())
		if report.Sources[0].Deleted != 0 || report.Sources[0].Partial {
			t.Fatalf("round %d must retain the target: %+v", round, report.Sources[0])
		}
		if got := countFor(t, store, "Sample.strm"); got != int64(round) {
			t.Fatalf("round %d count = %d, want %d", round, got, round)
		}
		if _, err := os.Stat(filepath.Join(root, "Sample.strm")); err != nil {
			t.Fatalf("round %d removed the target too early", round)
		}
	}
	report := runSource(t, lister, store, root, cleanupOptions())
	if report.Sources[0].Deleted != 1 {
		t.Fatalf("fourth sync did not delete exactly one target: %+v", report.Sources[0])
	}
	if _, err := os.Stat(filepath.Join(root, "Sample.strm")); !os.IsNotExist(err) {
		t.Fatal("qualified target was not removed")
	}
	if _, ok, _ := store.Lookup("/Movies", "Sample.strm"); ok {
		t.Fatal("deleted target kept its ownership row")
	}
	if _, err := os.Stat(filepath.Join(root, "NoHash.strm")); err != nil {
		t.Fatal("unrelated owned target was removed")
	}
	if epoch, _ := store.NextEpoch("/Movies"); epoch != 6 {
		t.Fatalf("unexpected round after deletion: %d", epoch)
	}
}

func TestHigherThresholdOnlyDelaysCleanup(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	opts := cleanupOptions()
	opts.MissingConfirmations = 4
	report := runSource(t, lister, store, root, opts)
	if report.Sources[0].Created != 4 {
		t.Fatalf("seed result: %+v", report.Sources[0])
	}
	removeRemote(lister, "Sample.mkv")
	for round := 1; round <= 4; round++ {
		report := runSource(t, lister, store, root, opts)
		if report.Sources[0].Deleted != 0 {
			t.Fatalf("round %d deleted below the configured threshold: %+v", round, report.Sources[0])
		}
		if got := countFor(t, store, "Sample.strm"); got != int64(round) {
			t.Fatalf("round %d count = %d", round, got)
		}
	}
	if report := runSource(t, lister, store, root, opts); report.Sources[0].Deleted != 1 {
		t.Fatalf("threshold 4 did not delete on the next sync: %+v", report.Sources[0])
	}
}

func TestReappearanceClearsCountsAndRefreshesMapping(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	seedStandard(t, lister, store, root)
	removeRemote(lister, "Sample.mkv")
	if report := runSource(t, lister, store, root, cleanupOptions()); report.Sources[0].ConfirmedMissing != 1 {
		t.Fatalf("missing observation not staged: %+v", report.Sources[0])
	}
	if got := countFor(t, store, "Sample.strm"); got != 1 {
		t.Fatalf("count after first missing round = %d", got)
	}
	// The source file reappears; the same target now skips and must clear the
	// committed evidence even before the rest of the round is known.
	lister.dirs["/Movies"] = append(lister.dirs["/Movies"], media("Sample.mkv", 10737418240, shaA))
	delete(lister.getErr, "/Movies/Sample.mkv")
	report := runSource(t, lister, store, root, cleanupOptions())
	if report.Sources[0].Skipped != 4 || report.Sources[0].Deleted != 0 {
		t.Fatalf("reappearance result: %+v", report.Sources[0])
	}
	o, ok, err := store.Lookup("/Movies", "Sample.strm")
	if err != nil || !ok || o.MissingCount != 0 || o.RemotePath != "/Movies/Sample.mkv" {
		t.Fatalf("reappearance did not refresh the row: %+v ok=%v err=%v", o, ok, err)
	}
}

func TestCleanupDisabledNeverCountsOrDeletes(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	opts := Options{PublicPrefix: "https://openlist.example.invalid", CleanupEnabled: false}
	report := runSource(t, lister, store, root, opts)
	if report.Sources[0].Created != 4 {
		t.Fatalf("seed result: %+v", report.Sources[0])
	}
	removeRemote(lister, "Sample.mkv")
	for round := 0; round < 5; round++ {
		report := runSource(t, lister, store, root, opts)
		if report.Sources[0].Deleted != 0 || report.Sources[0].ConfirmedMissing != 0 {
			t.Fatalf("disabled cleanup changed state: %+v", report.Sources[0])
		}
	}
	if got := countFor(t, store, "Sample.strm"); got != 0 {
		t.Fatalf("disabled cleanup accumulated counts: %d", got)
	}
	if _, err := os.Stat(filepath.Join(root, "Sample.strm")); err != nil {
		t.Fatal("disabled cleanup removed the target")
	}
}

func TestDryRunPreviewsCandidateWithoutChangingState(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	seedStandard(t, lister, store, root)
	removeRemote(lister, "Sample.mkv")
	for round := 0; round < 3; round++ {
		runSource(t, lister, store, root, cleanupOptions())
	}
	beforeBytes, err := os.ReadFile(filepath.Join(root, "Sample.strm"))
	if err != nil {
		t.Fatal(err)
	}
	dry := cleanupOptions()
	dry.DryRun = true
	report := runSource(t, lister, store, root, dry)
	if report.Sources[0].Candidates != 1 || report.Sources[0].Deleted != 0 {
		t.Fatalf("dry-run preview: %+v", report.Sources[0])
	}
	afterBytes, err := os.ReadFile(filepath.Join(root, "Sample.strm"))
	if err != nil || string(afterBytes) != string(beforeBytes) {
		t.Fatal("dry-run changed the previewed target")
	}
	if got := countFor(t, store, "Sample.strm"); got != 3 {
		t.Fatalf("dry-run changed committed counts: %d", got)
	}
	if _, ok, _ := store.Lookup("/Movies", "Sample.strm"); !ok {
		t.Fatal("dry-run removed ownership")
	}
	// The preview cannot become evidence: the next real sync is the one that
	// performs the deletion.
	if report := runSource(t, lister, store, root, cleanupOptions()); report.Sources[0].Deleted != 1 {
		t.Fatalf("real sync after preview did not converge: %+v", report.Sources[0])
	}
}

func TestDryRunObservedCandidateIsNotPreviewed(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	seedStandard(t, lister, store, root)
	removeRemote(lister, "Sample.mkv")
	for round := 0; round < 3; round++ {
		runSource(t, lister, store, root, cleanupOptions())
	}
	// The file reappears in the listing; a dry-run must not preview it as a
	// deletion candidate even though the committed count is still eligible.
	lister.dirs["/Movies"] = append(lister.dirs["/Movies"], media("Sample.mkv", 10737418240, shaA))
	delete(lister.getErr, "/Movies/Sample.mkv")
	dry := cleanupOptions()
	dry.DryRun = true
	report := runSource(t, lister, store, root, dry)
	if report.Sources[0].Candidates != 0 {
		t.Fatalf("observed target was previewed for deletion: %+v", report.Sources[0])
	}
	if report.Sources[0].Skipped != 4 {
		t.Fatalf("reappeared file was not skipped: %+v", report.Sources[0])
	}
}

func TestDeleteSyncFailureStopsCleanupAndRecoveryReclaims(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	seedStandard(t, lister, store, root)
	removeRemote(lister, "Sample.mkv")
	for round := 0; round < 3; round++ {
		runSource(t, lister, store, root, cleanupOptions())
	}
	failing := &LocalFS{syncDir: func(*securepath.Dir) error { return errors.New("injected sync failure") }}
	opts := cleanupOptions()
	opts.FS = failing
	report, err := New(lister, store, opts).Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("a durable-sync failure must fail the round")
	}
	if report.Sources[0].Deleted != 1 {
		t.Fatalf("actual unlink was not reported: %+v", report.Sources[0])
	}
	if _, err := os.Stat(filepath.Join(root, "Sample.strm")); !os.IsNotExist(err) {
		t.Fatal("unlink did not happen before the injected sync failure")
	}
	if _, ok, _ := store.Lookup("/Movies", "Sample.strm"); !ok {
		t.Fatal("failed round dropped ownership before a durable recovery")
	}
	if epoch, _ := store.NextEpoch("/Movies"); epoch != 5 {
		t.Fatalf("failed round advanced the epoch: %d", epoch)
	}
	// The release-equivalent failure-free restart must flush the absent target,
	// reclaim its row and commit normally.
	recovered := runSource(t, lister, store, root, cleanupOptions())
	if recovered.Sources[0].Reclaimed != 1 || recovered.Sources[0].Partial {
		t.Fatalf("recovery did not reclaim the absent target: %+v", recovered.Sources[0])
	}
	if _, ok, _ := store.Lookup("/Movies", "Sample.strm"); ok {
		t.Fatal("recovery left the ownership row behind")
	}
	if epoch, _ := store.NextEpoch("/Movies"); epoch != 6 {
		t.Fatalf("recovery did not commit its round: %d", epoch)
	}
}

func TestStagingResidualAndSymlinkArePreserved(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	seedStandard(t, lister, store, root)
	removeRemote(lister, "Sample.mkv")
	for round := 0; round < 3; round++ {
		runSource(t, lister, store, root, cleanupOptions())
	}
	staging := filepath.Join(root, stagingPrefix("Sample.strm")+"00000000000000000000000000000000"+updateStagingSuffix)
	if err := os.WriteFile(staging, []byte("residual"), 0o644); err != nil {
		t.Fatal(err)
	}
	report := runSource(t, lister, store, root, cleanupOptions())
	if report.Sources[0].Deleted != 0 || report.Sources[0].Retained == 0 {
		t.Fatalf("staging residual was not preserved: %+v", report.Sources[0])
	}
	if raw, err := os.ReadFile(staging); err != nil || string(raw) != "residual" {
		t.Fatalf("staging residual was changed: %q err=%v", raw, err)
	}
	if _, err := os.Stat(filepath.Join(root, "Sample.strm")); err != nil {
		t.Fatal("target was removed despite the staging residual")
	}
	// A symlink target must likewise be preserved.
	outside := filepath.Join(t.TempDir(), "sentinel")
	if err := os.WriteFile(outside, []byte("sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "Sample.strm")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "Sample.strm")); err != nil {
		t.Fatal(err)
	}
	report = runSource(t, lister, store, root, cleanupOptions())
	if report.Sources[0].Deleted != 0 {
		t.Fatalf("symlink target was deleted: %+v", report.Sources[0])
	}
	if raw, err := os.ReadFile(outside); err != nil || string(raw) != "sentinel" {
		t.Fatalf("symlink target changed: %q err=%v", raw, err)
	}
}

func TestLegacyUnmappedRowIsPreservedUntilObserved(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store, path := newStoreWithPath(t)
	seedStandard(t, lister, store, root)
	// Simulate a Slice 1 row that lost its remote mapping and carries an
	// unproven historical count.
	rawExec(t, path,
		`UPDATE ownership SET remote_path = '', missing_count = 5, missing_round = 0
		 WHERE source_key = '/Movies' AND relative_path = 'Sample.strm'`)
	removeRemote(lister, "Sample.mkv")
	report := runSource(t, lister, store, root, cleanupOptions())
	if report.Sources[0].Retained == 0 || report.Sources[0].Deleted != 0 {
		t.Fatalf("legacy unmapped row was not preserved: %+v", report.Sources[0])
	}
	if got := countFor(t, store, "Sample.strm"); got != 5 {
		t.Fatalf("legacy count changed: %d", got)
	}
	// A later observation safely fills the mapping and clears the count.
	removeListing(lister, "/Movies", "Sample.mkv")
	lister.dirs["/Movies"] = append(lister.dirs["/Movies"], media("Sample.mkv", 10737418240, shaA))
	delete(lister.getErr, "/Movies/Sample.mkv")
	runSource(t, lister, store, root, cleanupOptions())
	o, ok, err := store.Lookup("/Movies", "Sample.strm")
	if err != nil || !ok || o.RemotePath != "/Movies/Sample.mkv" || o.MissingCount != 0 {
		t.Fatalf("observation did not repair the legacy row: %+v ok=%v err=%v", o, ok, err)
	}
}

func TestSourceRootFailureFreezesCounts(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	seedStandard(t, lister, store, root)
	removeRemote(lister, "Sample.mkv")
	if report := runSource(t, lister, store, root, cleanupOptions()); countFor(t, store, "Sample.strm") != 1 {
		t.Fatalf("baseline count missing: %+v", report.Sources[0])
	}
	lister.getErr["/Movies"] = errors.New("root unavailable")
	report, err := New(lister, store, cleanupOptions()).Run(context.Background(), []Source{sourceFor(root)})
	if err == nil || !report.Sources[0].Partial {
		t.Fatalf("unreadable root must fail the round: %+v err=%v", report.Sources[0], err)
	}
	if got := countFor(t, store, "Sample.strm"); got != 1 {
		t.Fatalf("unreadable root changed counts: %d", got)
	}
	if _, err := os.Stat(filepath.Join(root, "Sample.strm")); err != nil {
		t.Fatal("unreadable root removed a target")
	}
}

func TestTargetGetFailureCountsButTypeConflictRetains(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	seedStandard(t, lister, store, root)
	removeRemote(lister, "Sample.mkv")
	if report := runSource(t, lister, store, root, cleanupOptions()); report.Sources[0].ConfirmedMissing != 1 {
		t.Fatalf("target read failure must still count: %+v", report.Sources[0])
	}
	// The same Get now reports a directory: type conflict, retained and the old
	// missing evidence is conservatively cleared by the successful read.
	delete(lister.getErr, "/Movies/Sample.mkv")
	report := runSource(t, lister, store, root, cleanupOptions())
	if report.Sources[0].Retained == 0 {
		t.Fatalf("type conflict was not reported: %+v", report.Sources[0])
	}
	if got := countFor(t, store, "Sample.strm"); got != 0 {
		t.Fatalf("type conflict did not clear the old evidence: %d", got)
	}
	if _, err := os.Stat(filepath.Join(root, "Sample.strm")); err != nil {
		t.Fatal("type conflict removed the target")
	}
}

func TestListFailureDoesNotAdvanceCounts(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	seedStandard(t, lister, store, root)
	removeRemote(lister, "Sample.mkv")
	if report := runSource(t, lister, store, root, cleanupOptions()); countFor(t, store, "Sample.strm") != 1 {
		t.Fatalf("baseline count: %+v", report.Sources[0])
	}
	lister.listErr["/Movies"] = errors.New("list failure")
	report, err := New(lister, store, cleanupOptions()).Run(context.Background(), []Source{sourceFor(root)})
	if err == nil || !report.Sources[0].Partial {
		t.Fatalf("list failure must fail the round: %+v", report.Sources[0])
	}
	if got := countFor(t, store, "Sample.strm"); got != 1 {
		t.Fatalf("failed round advanced counts: %d", got)
	}
}

func TestMultiSourceEvidenceIsIsolated(t *testing.T) {
	lister := standardTree()
	rootA := t.TempDir()
	rootB := t.TempDir()
	store := newStore(t)
	sourceA := Source{Index: 0, Key: "source-a", Remote: "/Movies", Output: rootA}
	sourceB := Source{Index: 1, Key: "source-b", Remote: "/Movies", Output: rootB}
	if report, err := New(lister, store, cleanupOptions()).Run(context.Background(), []Source{sourceA, sourceB}); err != nil {
		t.Fatalf("seed: %v (%+v)", err, report)
	}
	removeRemote(lister, "Sample.mkv")
	// Only source A completes missing rounds; source B must not inherit any of
	// that evidence through the shared store.
	for round := 0; round < 2; round++ {
		if report, err := New(lister, store, cleanupOptions()).Run(context.Background(), []Source{sourceA}); err != nil {
			t.Fatalf("source A round: %v (%+v)", err, report)
		}
	}
	if report, err := New(lister, store, cleanupOptions()).Run(context.Background(), []Source{sourceB}); err != nil {
		t.Fatalf("source B round: %v (%+v)", err, report)
	}
	countA, okA, err := store.Lookup("source-a", "Sample.strm")
	if err != nil || !okA || countA.MissingCount != 2 {
		t.Fatalf("source A count = %+v ok=%v err=%v", countA, okA, err)
	}
	countB, okB, err := store.Lookup("source-b", "Sample.strm")
	if err != nil || !okB || countB.MissingCount != 1 {
		t.Fatalf("source B borrowed source A evidence: %+v ok=%v err=%v", countB, okB, err)
	}
	if countB.MissingRound == countA.MissingRound {
		t.Fatal("source B missing round must reflect only its own completed rounds")
	}
}

func TestFixedBoundaryMismatchIsRetained(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store, path := newStoreWithPath(t)
	seedStandard(t, lister, store, root)
	// The target is gone locally, so no traversal conflict is raised; the
	// stored absolute path no longer matches the configured boundary.
	if err := os.Remove(filepath.Join(root, "Sample.strm")); err != nil {
		t.Fatal(err)
	}
	rawExec(t, path,
		`UPDATE ownership SET absolute_path = '/somewhere/else/Sample.strm', missing_count = 5, missing_round = 1
		 WHERE source_key = '/Movies' AND relative_path = 'Sample.strm'`)
	removeRemote(lister, "Sample.mkv")
	report := runSource(t, lister, store, root, cleanupOptions())
	if report.Sources[0].Deleted != 0 || report.Sources[0].Retained == 0 {
		t.Fatalf("boundary mismatch was not retained: %+v", report.Sources[0])
	}
	if _, err := os.Stat(filepath.Join(root, "Sample.strm")); !os.IsNotExist(err) {
		t.Fatal("boundary mismatch recreated or removed a local target")
	}
}

func TestReappearanceClearsEvidenceBeforeLaterFailure(t *testing.T) {
	// An observed source file clears the old missing evidence immediately, even
	// when the same target then fails local processing and the round does not
	// commit.
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	seedStandard(t, lister, store, root)
	removeRemote(lister, "Sample.mkv")
	if report := runSource(t, lister, store, root, cleanupOptions()); countFor(t, store, "Sample.strm") != 1 {
		t.Fatalf("baseline count: %+v", report.Sources[0])
	}
	lister.dirs["/Movies"] = append(lister.dirs["/Movies"], media("Sample.mkv", 10737418240, shaA))
	delete(lister.getErr, "/Movies/Sample.mkv")
	if err := os.WriteFile(filepath.Join(root, "Sample.strm"), []byte("garbage-not-a-strm\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := New(lister, store, cleanupOptions()).Run(context.Background(), []Source{sourceFor(root)})
	if err == nil || !report.Sources[0].Partial || report.Sources[0].Conflicts == 0 {
		t.Fatalf("invalid owned target must fail the round: %+v err=%v", report.Sources[0], err)
	}
	if got := countFor(t, store, "Sample.strm"); got != 0 {
		t.Fatalf("accepted reappearance did not clear the count before failure: %d", got)
	}
	if epoch, _ := store.NextEpoch("/Movies"); epoch != 3 {
		t.Fatalf("failed round advanced the epoch: %d", epoch)
	}
}

// missingThenPresentConfirmer reports one missing observation and then a
// present target on the cleanup re-check, reproducing a remote that changes
// between the two calls in the same round.
type missingThenPresentConfirmer struct{ calls int }

func (c *missingThenPresentConfirmer) Confirm(context.Context, string, string) openlist.AbsenceResult {
	c.calls++
	if c.calls == 1 {
		return openlist.AbsenceResult{Status: openlist.AbsenceMissing, Reason: openlist.ReasonTargetUnread}
	}
	return openlist.AbsenceResult{Status: openlist.AbsencePresent, Reason: openlist.ReasonTargetTypeConflict}
}

func TestCleanupRecheckPresentCancelsStagedMissing(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	seedStandard(t, lister, store, root)
	removeRemote(lister, "Sample.mkv")
	for round := 0; round < 3; round++ {
		runSource(t, lister, store, root, cleanupOptions())
	}
	if got := countFor(t, store, "Sample.strm"); got != 3 {
		t.Fatalf("qualification count = %d", got)
	}
	confirmer := &missingThenPresentConfirmer{}
	opts := cleanupOptions()
	opts.Confirmer = confirmer
	report := runSource(t, lister, store, root, opts)
	if confirmer.calls < 2 {
		t.Fatalf("cleanup did not re-check the candidate: calls=%d", confirmer.calls)
	}
	if report.Sources[0].Deleted != 0 || report.Sources[0].Retained == 0 {
		t.Fatalf("present re-check result: %+v", report.Sources[0])
	}
	if got := countFor(t, store, "Sample.strm"); got != 0 {
		t.Fatalf("staged missing survived a present re-check: %d", got)
	}
	if _, err := os.Stat(filepath.Join(root, "Sample.strm")); err != nil {
		t.Fatal("present re-check removed the target")
	}
}

func TestNewPathFailureDoesNotCleanupOldPath(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	seedStandard(t, lister, store, root)
	removeRemote(lister, "Sample.mkv")
	for round := 0; round < 3; round++ {
		runSource(t, lister, store, root, cleanupOptions())
	}
	// A new path appears but its parent cannot be created because a regular
	// file already occupies the directory name. The round must fail before any
	// cleanup, so the qualified old path stays.
	if err := os.WriteFile(filepath.Join(root, "NewDir"), []byte("blocker"), 0o644); err != nil {
		t.Fatal(err)
	}
	lister.dirs["/Movies"] = append(lister.dirs["/Movies"], dir("NewDir"))
	lister.dirs["/Movies/NewDir"] = []openlist.Entry{media("Moved.mkv", 5, shaB)}
	report, err := New(lister, store, cleanupOptions()).Run(context.Background(), []Source{sourceFor(root)})
	if err == nil || !report.Sources[0].Partial {
		t.Fatalf("new-path failure must fail the round: %+v err=%v", report.Sources[0], err)
	}
	if report.Sources[0].Deleted != 0 {
		t.Fatalf("new-path failure still cleaned the old path: %+v", report.Sources[0])
	}
	if _, statErr := os.Stat(filepath.Join(root, "Sample.strm")); statErr != nil {
		t.Fatal("old path was removed after the new path failed")
	}
	if got := countFor(t, store, "Sample.strm"); got != 3 {
		t.Fatalf("failed round changed the old count: %d", got)
	}
	if epoch, _ := store.NextEpoch("/Movies"); epoch != 5 {
		t.Fatalf("failed round advanced the epoch: %d", epoch)
	}
}

func TestCommittedEvidenceIsNotConsumedByCurrentRound(t *testing.T) {
	// Two missing rounds leave the committed count at two. The third full round
	// must not use its own uncommitted observation to reach three and delete.
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	seedStandard(t, lister, store, root)
	removeRemote(lister, "Sample.mkv")
	for round := 0; round < 2; round++ {
		runSource(t, lister, store, root, cleanupOptions())
	}
	report := runSource(t, lister, store, root, cleanupOptions())
	if report.Sources[0].Deleted != 0 {
		t.Fatalf("uncommitted round evidence was consumed: %+v", report.Sources[0])
	}
	if got := countFor(t, store, "Sample.strm"); got != 3 {
		t.Fatalf("third round count = %d, want 3", got)
	}
}
