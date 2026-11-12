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
	"strm-generator-go/internal/storage"
)

// recordingSink collects facts in the order they occurred.
type recordingSink struct {
	facts []Fact
}

func (r *recordingSink) Record(fact Fact) {
	r.facts = append(r.facts, fact)
}

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
func runSourceWithSink(t *testing.T, lister *fakeLister, store *storage.Store, root string, opts Options, sink *recordingSink) *Report {
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

// commitReportStore wraps a real store and replaces only the commit result so
// the committed-but-unclean window is deterministic.
type commitReportStore struct {
	*storage.Store
	result storage.CommitResult
	err    error
}

func (f *commitReportStore) CommitRound(_ context.Context, _ string, _ int64, _ time.Time) (storage.CommitResult, error) {
	return f.result, f.err
}

func TestFactsReportCommittedAdvancesWhenWorkspaceCleanupFails(t *testing.T) {
	lister := standardTree()
	root := t.TempDir()
	realStore := newStore(t)
	sink := &recordingSink{}
	seedStandard(t, lister, realStore, root)

	store := &commitReportStore{
		Store: realStore,
		result: storage.CommitResult{Committed: true, Advances: []storage.MissingAdvance{{
			RelativePath: "Sample.strm", PriorCount: 1, NewCount: 2, Reason: storage.MissingReasonSourceTarget,
		}}},
		err: errors.New("scratch cleanup failed after commit"),
	}
	sink.facts = nil
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
