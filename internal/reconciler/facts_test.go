package reconciler

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"strm-generator-go/internal/openlist"
	"strm-generator-go/internal/pathmap"
	"strm-generator-go/internal/securepath"
	"strm-generator-go/internal/storage"
)

// recordingSink collects facts in the order they occurred. Missing-count
// advances arrive as provisional records while the commit transaction runs;
// the commit and discard calls are tracked so tests can assert that an
// uncommitted candidate is never published.
type recordingSink struct {
	facts    []Fact
	commits  int
	discards int
}

func (r *recordingSink) Record(fact Fact) {
	r.facts = append(r.facts, fact)
}

func (r *recordingSink) CommitStaged()  { r.commits++ }
func (r *recordingSink) DiscardStaged() { r.discards++ }

func (r *recordingSink) kinds() []FactKind {
	out := make([]FactKind, 0, len(r.facts))
	for _, fact := range r.facts {
		out = append(out, fact.Kind)
	}
	return out
}

func factKindsOf(sink *recordingSink, kind FactKind) []Fact {
	var out []Fact
	for _, fact := range sink.facts {
		if fact.Kind == kind {
			out = append(out, fact)
		}
	}
	return out
}

// runSourceWithSink runs one source with a fact sink attached.
func runSourceWithSink(t *testing.T, lister *fakeLister, store OwnershipStore, root string, opts Options, sink *recordingSink) *Report {
	t.Helper()
	opts.Facts = sink
	report, err := New(lister, store, opts).Run(context.Background(), []Source{sourceFor(root)})
	if report == nil {
		t.Fatalf("run returned no report: %v", err)
	}
	return report
}

func relPathsOf(facts []Fact) []string {
	out := make([]string, 0, len(facts))
	for _, fact := range facts {
		out = append(out, fact.RelativePath)
	}
	return out
}

func TestFactsCaptureCreatedEventsOnlyForRealOperations(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	sink := &recordingSink{}
	report := runSourceWithSink(t, lister, store, root, cleanupOptions(), sink)
	if report.Sources[0].Created != 4 {
		t.Fatalf("seed result: %+v", report.Sources[0])
	}
	created := factKindsOf(sink, FactCreated)
	if len(created) != 4 {
		t.Fatalf("created facts = %d, want 4 (%v)", len(created), sink.kinds())
	}
	for _, fact := range created {
		if fact.SourceIndex != 0 {
			t.Fatalf("created fact carries the wrong source index: %+v", fact)
		}
		if !strings.HasSuffix(fact.RelativePath, ".strm") || strings.Contains(fact.RelativePath, "notes") {
			t.Fatalf("created fact carries a non-STRM relative path: %+v", fact)
		}
	}
	found := map[string]bool{}
	for _, fact := range created {
		found[fact.RelativePath] = true
	}
	for _, want := range []string{"Sample.strm", "NoHash.strm", filepath.Join("科幻", "A #100%.strm")} {
		if !found[want] {
			t.Fatalf("missing created fact for %s: %v", want, relPathsOf(created))
		}
	}
}

func TestFactsCaptureUpdatedAfterRemoteChange(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	sink := &recordingSink{}
	runSourceWithSink(t, lister, store, root, cleanupOptions(), sink)
	sink.facts = nil

	// Change the hash of one listed file: the STRM content changes.
	entries := append([]openlist.Entry(nil), lister.dirs["/Movies"]...)
	for i := range entries {
		if entries[i].Name == "Sample.mkv" {
			entries[i].SHA1 = strings.Repeat("c", 40)
			entries[i].HasSHA = true
		}
	}
	lister.dirs["/Movies"] = entries
	report := runSourceWithSink(t, lister, store, root, cleanupOptions(), sink)
	if report.Sources[0].Updated != 1 {
		t.Fatalf("updated result: %+v", report.Sources[0])
	}
	updated := factKindsOf(sink, FactUpdated)
	if len(updated) != 1 || updated[0].RelativePath != "Sample.strm" {
		t.Fatalf("updated facts: %+v", updated)
	}
}

func TestFactsCaptureMissingAdvanceDeletionAndClear(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	sink := &recordingSink{}
	seedStandard(t, lister, store, root)
	sink.facts = nil

	removeRemote(lister, "NoHash.mp4")
	for round := int64(1); round <= 3; round++ {
		report := runSourceWithSink(t, lister, store, root, cleanupOptions(), sink)
		if report.Sources[0].Deleted != 0 {
			t.Fatalf("round %d deleted too early: %+v", round, report.Sources[0])
		}
		advances := factKindsOf(sink, FactMissingAdvanced)
		if len(advances) != 1 {
			t.Fatalf("round %d advances: %+v", round, advances)
		}
		a := advances[0]
		if a.RelativePath != "NoHash.strm" || a.PriorCount != round-1 || a.NewCount != round {
			t.Fatalf("round %d advance fact: %+v", round, a)
		}
		if a.Reason != FactReasonSourceTarget {
			t.Fatalf("advance reason: %q", a.Reason)
		}
		sink.facts = nil
	}

	// Restoring the remote file clears the committed count. The clear is a
	// durable fact captured in the original observation window.
	restore := append([]openlist.Entry(nil), standardTree().dirs["/Movies"]...)
	lister.dirs["/Movies"] = restore
	delete(lister.getErr, "/Movies/NoHash.mp4")
	report := runSourceWithSink(t, lister, store, root, cleanupOptions(), sink)
	if report.Sources[0].Partial {
		t.Fatalf("restored round failed: %+v", report.Sources[0])
	}
	cleared := factKindsOf(sink, FactMissingCleared)
	if len(cleared) != 1 || cleared[0].RelativePath != "NoHash.strm" || cleared[0].PriorCount != 3 {
		t.Fatalf("clear facts: %+v", cleared)
	}
	if got := countFor(t, store, "NoHash.strm"); got != 0 {
		t.Fatalf("count after restore = %d, want 0", got)
	}
}

func TestFactsCaptureDeletionAtThreshold(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	sink := &recordingSink{}
	seedStandard(t, lister, store, root)
	sink.facts = nil

	removeRemote(lister, "Sample.mkv")
	for round := 1; round <= 3; round++ {
		runSourceWithSink(t, lister, store, root, cleanupOptions(), sink)
		sink.facts = nil
	}
	report := runSourceWithSink(t, lister, store, root, cleanupOptions(), sink)
	if report.Sources[0].Deleted != 1 {
		t.Fatalf("deletion round: %+v", report.Sources[0])
	}
	deleted := factKindsOf(sink, FactDeleted)
	if len(deleted) != 1 {
		t.Fatalf("deleted facts: %+v", deleted)
	}
	d := deleted[0]
	if d.RelativePath != "Sample.strm" || d.PriorCount != 3 {
		t.Fatalf("deleted fact: %+v", d)
	}
	if d.Reason != FactReasonSourceTarget {
		t.Fatalf("deleted reason: %q", d.Reason)
	}
}

func TestFactsCaptureExcludedExtensionReason(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	sink := &recordingSink{}
	seedStandard(t, lister, store, root)
	sink.facts = nil

	// Keep only .mp4: every .mkv target stays on disk with a source file that
	// still reads successfully as a regular file, so its staged reason is the
	// exclusion rather than a type conflict.
	lister.getFile = map[string]bool{
		"/Movies/Sample.mkv":         true,
		"/Movies/科幻/A #100%.mkv":     true,
		"/Movies/科幻/InvalidHash.mkv": true,
	}
	opts := cleanupOptions()
	opts.MediaExtensions = pathmap.NewMediaSet([]string{".mp4"})
	report := runSourceWithSink(t, lister, store, root, opts, sink)
	if report.Sources[0].Partial || report.Sources[0].ConfirmedMissing == 0 {
		t.Fatalf("excluded round: %+v", report.Sources[0])
	}
	advances := factKindsOf(sink, FactMissingAdvanced)
	for _, a := range advances {
		if a.Reason != FactReasonExcludedExtension {
			t.Fatalf("advance reason: %+v", a)
		}
	}
	if len(advances) < 3 {
		t.Fatalf("excluded advances: %+v", advances)
	}
}

// commitReportStore wraps a real store and injects a post-commit cleanup
// failure so the committed-but-unclean window is deterministic. The commit
// itself and its advance facts stay real.
type commitReportStore struct {
	*storage.Store
	failCleanup bool
}

func (f *commitReportStore) CommitRound(ctx context.Context, key string, epoch int64, at time.Time, advances storage.AdvanceSink) (storage.CommitResult, error) {
	result, err := f.Store.CommitRound(ctx, key, epoch, at, advances)
	if err == nil && f.failCleanup {
		err = errors.New("injected workspace cleanup failure")
	}
	return result, err
}

// failingClearStore wraps a real store and fails the private scratch cleanup
// of ClearMissing after the main-database clear succeeded, mirroring the real
// storage behavior of returning the durable result together with the error.
type failingClearStore struct {
	*storage.Store
	failOn string
}

func (f *failingClearStore) ClearMissing(ctx context.Context, key, rel string) (storage.ClearResult, error) {
	result, err := f.Store.ClearMissing(ctx, key, rel)
	if err == nil && rel == f.failOn {
		err = errors.New("injected scratch cleanup failure")
	}
	return result, err
}

func TestFactsReportCommittedAdvancesWhenWorkspaceCleanupFails(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	realStore := newStore(t)
	sink := &recordingSink{}
	seedStandard(t, lister, realStore, root)

	// One missing round commits normally: the count reaches 1.
	removeRemote(lister, "NoHash.mp4")
	if report := runSourceWithSink(t, lister, realStore, root, cleanupOptions(), sink); report.Sources[0].Partial {
		t.Fatalf("first missing round: %+v", report.Sources[0])
	}
	sink.facts = nil

	// The second round commits but its workspace cleanup fails afterwards.
	store := &commitReportStore{Store: realStore, failCleanup: true}
	opts := cleanupOptions()
	opts.Facts = sink
	report, err := New(lister, store, opts).Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("workspace cleanup failure must remain a round failure")
	}
	res := report.Sources[0]
	if !res.Partial {
		t.Fatalf("result: %+v", res)
	}
	// The committed round must never be diagnosed as uncommitted.
	joined := strings.Join(res.Diagnostics, "\n")
	if !strings.Contains(joined, "round committed; the round workspace could not be cleared") {
		t.Fatalf("diagnostics: %v", res.Diagnostics)
	}
	if strings.Contains(joined, "round commit failed; source round not committed") {
		t.Fatalf("committed round misreported as uncommitted: %v", res.Diagnostics)
	}
	advances := factKindsOf(sink, FactMissingAdvanced)
	if len(advances) != 1 || advances[0].PriorCount != 1 || advances[0].NewCount != 2 {
		t.Fatalf("advance facts after commit: %+v", advances)
	}
	if got := countFor(t, realStore, "NoHash.strm"); got != 2 {
		t.Fatalf("durable count after committed round = %d, want 2", got)
	}
}

// B review B-02: a normal I/O failure after an owned update was already
// published must keep the update fact and the new bytes; the round still
// fails. This uses the existing LocalFS injection point.
func TestUpdatePublishedBeforeFailureKeepsFactAndBytes(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	seedStandard(t, lister, store, root)
	entries := append([]openlist.Entry(nil), lister.dirs["/Movies"]...)
	for i := range entries {
		if entries[i].Name == "Sample.mkv" {
			entries[i].SHA1 = strings.Repeat("c", 40)
			entries[i].HasSHA = true
		}
	}
	lister.dirs["/Movies"] = entries
	sink := &recordingSink{}
	opts := cleanupOptions()
	opts.FS = LocalFS{afterUpdate: func(_ *securepath.Dir, _ string) error {
		return errors.New("post-publish I/O failure")
	}}
	report := runSourceWithSink(t, lister, store, root, opts, sink)
	raw, err := (LocalFS{}).ReadFile(root, "Sample.strm")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), strings.Repeat("c", 40)) || !report.Sources[0].Partial {
		t.Fatalf("failure window not reached: partial=%v", report.Sources[0].Partial)
	}
	if report.Sources[0].Updated != 1 || report.Sources[0].Errors == 0 {
		t.Fatalf("published update must be counted and still fail: %+v", report.Sources[0])
	}
	facts := factKindsOf(sink, FactUpdated)
	if len(facts) != 1 || facts[0].RelativePath != "Sample.strm" {
		t.Fatalf("published update fact lost after post-publication failure: %+v", facts)
	}
	// A later normal round converges: the update is re-registered cleanly.
	sink.facts = nil
	opts.FS = LocalFS{}
	report = runSourceWithSink(t, lister, store, root, opts, sink)
	if report.Sources[0].Partial {
		t.Fatalf("recovery round failed: %+v", report.Sources[0])
	}
	if _, ok, err := store.Lookup("/Movies", "Sample.strm"); err != nil || !ok {
		t.Fatalf("ownership lost after recovery: ok=%v err=%v", ok, err)
	}
}

// B review B-02: a directory-durability failure after Publish linked the new
// target keeps the creation fact and the bytes; the round still fails.
func TestPublishLinkedBeforeDirectoryFailureKeepsFact(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	sink := &recordingSink{}
	opts := cleanupOptions()
	opts.FS = LocalFS{syncDir: func(_ *securepath.Dir) error {
		return errors.New("post-link directory sync failure")
	}}
	report := runSourceWithSink(t, lister, store, root, opts, sink)
	if !report.Sources[0].Partial || report.Sources[0].Errors == 0 {
		t.Fatalf("failure not reported: %+v", report.Sources[0])
	}
	if report.Sources[0].Created != 4 {
		t.Fatalf("published creations must be counted: %+v", report.Sources[0])
	}
	if facts := factKindsOf(sink, FactCreated); len(facts) != 4 {
		t.Fatalf("created facts lost after directory failure: %d", len(facts))
	}
	raw, err := (LocalFS{}).ReadFile(root, "Sample.strm")
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) == 0 {
		t.Fatal("published bytes missing")
	}
}

// B review B-03: a committed N -> 0 reset stands on its own. When the private
// scratch cleanup fails right after the durable clear, the fact must still be
// reported and the round still fails; the same applies to every clear window.
func TestClearFactSurvivesScratchFailureInEveryWindow(t *testing.T) {
	buildWithCount := func(t *testing.T) (*fakeLister, *storage.Store, string, *recordingSink) {
		lister := standardTree()
		root := t.TempDir()
		store := newStore(t)
		seedStandard(t, lister, store, root)
		removeRemote(lister, "NoHash.mp4")
		sink := &recordingSink{}
		runSourceWithSink(t, lister, store, root, cleanupOptions(), sink)
		return lister, store, root, sink
	}

	// Window 1: noteObserved — the target is listed and read again.
	lister, realStore, root, sink := buildWithCount(t)
	sink.facts = nil
	restore := append([]openlist.Entry(nil), standardTree().dirs["/Movies"]...)
	lister.dirs["/Movies"] = restore
	delete(lister.getErr, "/Movies/NoHash.mp4")
	store := &failingClearStore{Store: realStore, failOn: "NoHash.strm"}
	report := runSourceWithSink(t, lister, store, root, cleanupOptions(), sink)
	if !report.Sources[0].Partial {
		t.Fatalf("window 1 must still fail: %+v", report.Sources[0])
	}
	cleared := factKindsOf(sink, FactMissingCleared)
	if len(cleared) != 1 || cleared[0].PriorCount != 1 {
		t.Fatalf("window 1 clear facts: %+v", cleared)
	}
	if row, ok, err := realStore.Lookup("/Movies", "NoHash.strm"); err != nil || !ok || row.MissingCount != 0 {
		t.Fatalf("window 1 durable count: %+v ok=%v err=%v", row, ok, err)
	}

	// Window 2: confirmMissing — the target is never observed because the
	// listing entry is gone while the exact Get still reads a regular file.
	lister, realStore, root, sink = buildWithCount(t)
	sink.facts = nil
	removeListing(lister, "/Movies", "NoHash.mkv")
	delete(lister.getErr, "/Movies/NoHash.mp4")
	lister.getFile = map[string]bool{"/Movies/NoHash.mp4": true}
	store = &failingClearStore{Store: realStore, failOn: "NoHash.strm"}
	report = runSourceWithSink(t, lister, store, root, cleanupOptions(), sink)
	if !report.Sources[0].Partial {
		t.Fatalf("window 2 must still fail: %+v", report.Sources[0])
	}
	cleared = factKindsOf(sink, FactMissingCleared)
	if len(cleared) != 1 || cleared[0].PriorCount != 1 {
		t.Fatalf("window 2 clear facts: %+v", cleared)
	}
	if row, ok, err := realStore.Lookup("/Movies", "NoHash.strm"); err != nil || !ok || row.MissingCount != 0 {
		t.Fatalf("window 2 durable count: %+v ok=%v err=%v", row, ok, err)
	}

	// Window 3: deleteCandidate re-check — the committed count reached the
	// threshold but the target reads again during the re-check.
	lister, realStore, root, sink = buildWithCount(t)
	runSourceWithSink(t, lister, realStore, root, cleanupOptions(), sink)
	runSourceWithSink(t, lister, realStore, root, cleanupOptions(), sink)
	if got := countFor(t, realStore, "NoHash.strm"); got != 3 {
		t.Fatalf("threshold setup count = %d, want 3", got)
	}
	sink.facts = nil
	removeListing(lister, "/Movies", "NoHash.mkv")
	delete(lister.getErr, "/Movies/NoHash.mp4")
	lister.getFile = map[string]bool{"/Movies/NoHash.mp4": true}
	store = &failingClearStore{Store: realStore, failOn: "NoHash.strm"}
	report = runSourceWithSink(t, lister, store, root, cleanupOptions(), sink)
	if !report.Sources[0].Partial {
		t.Fatalf("window 3 must still fail: %+v", report.Sources[0])
	}
	cleared = factKindsOf(sink, FactMissingCleared)
	if len(cleared) != 1 || cleared[0].PriorCount != 3 {
		t.Fatalf("window 3 clear facts: %+v", cleared)
	}
	if row, ok, err := realStore.Lookup("/Movies", "NoHash.strm"); err != nil || !ok || row.MissingCount != 0 {
		t.Fatalf("window 3 durable count: %+v ok=%v err=%v", row, ok, err)
	}
}

func TestFactsAreAbsentInDryRun(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	store := newStore(t)
	sink := &recordingSink{}
	opts := cleanupOptions()
	opts.DryRun = true
	report, err := New(lister, store, opts).Run(context.Background(), []Source{sourceFor(root)})
	if err != nil {
		t.Fatal(err)
	}
	if report.Sources[0].Created == 0 {
		t.Fatalf("dry-run preview: %+v", report.Sources[0])
	}
	if len(sink.facts) != 0 {
		t.Fatalf("dry-run must not record facts: %v", sink.kinds())
	}
}
