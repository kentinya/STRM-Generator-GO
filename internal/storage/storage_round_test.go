package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// stageMissing stages one pending missing observation for a test round.
func stageMissing(t *testing.T, ctx context.Context, s *Store, sourceKey, rel string) {
	t.Helper()
	if err := s.AddPendingMissing(ctx, sourceKey, []PendingMissingEntry{{RelativePath: rel, Reason: MissingReasonSourceTarget}}); err != nil {
		t.Fatal(err)
	}
}

// recordingAdvances is a test AdvanceSink that separates provisional records
// from committed ones so tests can assert the publish and discard protocol.
type recordingAdvances struct {
	staged    []MissingAdvance
	committed []MissingAdvance
	commits   int
	discards  int
}

func (r *recordingAdvances) StageAdvance(a MissingAdvance) { r.staged = append(r.staged, a) }
func (r *recordingAdvances) CommitAdvances() {
	r.committed = append(r.committed, r.staged...)
	r.staged = nil
	r.commits++
}
func (r *recordingAdvances) DiscardAdvances() { r.staged = nil; r.discards++ }

// commitRound commits one test round and fails the test on an error.
func commitRound(t *testing.T, ctx context.Context, s *Store, sourceKey string, epoch int64, at time.Time) {
	t.Helper()
	if _, err := s.CommitRound(ctx, sourceKey, epoch, at, nil); err != nil {
		t.Fatal(err)
	}
}

const legacySchema = `
CREATE TABLE ownership (
	source_key     TEXT    NOT NULL,
	relative_path  TEXT    NOT NULL,
	absolute_path  TEXT    NOT NULL,
	created_round  INTEGER NOT NULL,
	missing_count  INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (source_key, relative_path)
);
CREATE TABLE rounds (
	source_key   TEXT    NOT NULL PRIMARY KEY,
	epoch        INTEGER NOT NULL,
	committed_at TEXT    NOT NULL
);
`

func createLegacyDatabase(t *testing.T, path, script string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(legacySchema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if script != "" {
		if _, err := db.Exec(script); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestUpgradePreservesLegacyOwnershipAndRounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "strmgen.db")
	createLegacyDatabase(t, path,
		`INSERT INTO ownership VALUES ('source', 'A.strm', '/out/A.strm', 7, 5);
		 INSERT INTO rounds VALUES ('source', 7, 'legacy');`)

	s, err := Open(path)
	if err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	defer s.Close()
	if !s.PreciseMappingAvailable() {
		t.Fatal("upgraded store must report a precise mapping capability")
	}
	o, ok, err := s.Lookup("source", "A.strm")
	if err != nil || !ok {
		t.Fatalf("legacy row lost: ok=%v err=%v", ok, err)
	}
	if o.RemotePath != "" || o.MissingCount != 5 || o.MissingRound != 0 {
		t.Fatalf("legacy row transformed unexpectedly: %+v", o)
	}
	// A legacy count carries no proven missing round and can never qualify.
	eligible, err := s.ListEligibleAfter(context.Background(), "source", "", 3, 200)
	if err != nil || len(eligible) != 0 {
		t.Fatalf("legacy count became eligible: %v %v", eligible, err)
	}
	if count, err := s.CountUnmappedMissing(context.Background(), "source"); err != nil || count != 1 {
		t.Fatalf("unmapped legacy count = %d, err=%v", count, err)
	}
	// A later normal observation may safely refresh the mapping and clear the
	// unproven count.
	if err := s.RecordOwnership(context.Background(), Ownership{
		SourceKey: "source", RelativePath: "A.strm", AbsolutePath: "/out/A.strm",
		RemotePath: "/remote/A.mkv",
	}, 8); err != nil {
		t.Fatal(err)
	}
	o, _, err = s.Lookup("source", "A.strm")
	if err != nil || o.RemotePath != "/remote/A.mkv" || o.MissingCount != 0 {
		t.Fatalf("observation did not refresh legacy row: %+v err=%v", o, err)
	}
	if epoch, err := s.NextEpoch("source"); err != nil || epoch != 8 {
		t.Fatalf("legacy round was not preserved: epoch=%d err=%v", epoch, err)
	}
}

func TestUpgradeIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strmgen.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.RecordOwnership(context.Background(), Ownership{
		SourceKey: "source", RelativePath: "A.strm", AbsolutePath: "/out/A.strm", RemotePath: "/remote/A.mkv",
	}, 1); err != nil {
		t.Fatal(err)
	}
	first.Close()
	second, err := Open(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer second.Close()
	if o, ok, err := second.Lookup("source", "A.strm"); err != nil || !ok || o.RemotePath != "/remote/A.mkv" {
		t.Fatalf("row lost on reopen: %+v ok=%v err=%v", o, ok, err)
	}
}

func TestUnknownSchemaIsRejectedNotRebuilt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strmgen.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE foreign_table (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Open(path); err == nil {
		t.Fatal("an unknown schema must be rejected instead of rebuilt")
	}
	if _, err := OpenReadOnly(path); err == nil {
		t.Fatal("an unknown read-only schema must be rejected")
	}
}

func TestIncompleteSchemaIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strmgen.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(legacySchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE rounds`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := Open(path); err == nil {
		t.Fatal("a partial schema must be rejected")
	}
}

func TestMissingCountsCommitAtMostOncePerEpoch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strmgen.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.RecordOwnership(ctx, Ownership{
		SourceKey: "source", RelativePath: "A.strm", AbsolutePath: "/out/A.strm", RemotePath: "/remote/A.mkv",
	}, 1); err != nil {
		t.Fatal(err)
	}
	count := func() int64 {
		t.Helper()
		o, ok, err := s.Lookup("source", "A.strm")
		if err != nil || !ok {
			t.Fatalf("lookup: ok=%v err=%v", ok, err)
		}
		return o.MissingCount
	}
	if err := s.ResetRound(ctx, "source"); err != nil {
		t.Fatal(err)
	}
	stageMissing(t, ctx, s, "source", "A.strm")
	commitRound(t, ctx, s, "source", 1, time.Unix(0, 0))
	if got := count(); got != 1 {
		t.Fatalf("count after first epoch = %d, want 1", got)
	}
	// A repeated commit of the same epoch must not count twice.
	stageMissing(t, ctx, s, "source", "A.strm")
	commitRound(t, ctx, s, "source", 1, time.Unix(1, 0))
	if got := count(); got != 1 {
		t.Fatalf("duplicate epoch counted again: %d", got)
	}
	// A stale commit can never overwrite the newer round or count again.
	stageMissing(t, ctx, s, "source", "A.strm")
	commitRound(t, ctx, s, "source", 2, time.Unix(2, 0))
	if got := count(); got != 2 {
		t.Fatalf("second epoch count = %d, want 2", got)
	}
	stageMissing(t, ctx, s, "source", "A.strm")
	commitRound(t, ctx, s, "source", 1, time.Unix(3, 0))
	if got := count(); got != 2 {
		t.Fatalf("stale epoch changed the count: %d", got)
	}
	if epoch, err := s.NextEpoch("source"); err != nil || epoch != 3 {
		t.Fatalf("stale commit changed the round: epoch=%d err=%v", epoch, err)
	}
}

func TestMissingCountOverflowNeverWraps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strmgen.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if _, err := s.db.Exec(
		`INSERT INTO ownership (source_key, relative_path, absolute_path, remote_path, created_round, missing_count, missing_round)
		 VALUES ('source', 'A.strm', '/out/A.strm', '/remote/A.mkv', 1, ?, 0)`, maxInt64); err != nil {
		t.Fatal(err)
	}
	stageMissing(t, ctx, s, "source", "A.strm")
	commitRound(t, ctx, s, "source", 1, time.Unix(0, 0))
	o, _, err := s.Lookup("source", "A.strm")
	if err != nil {
		t.Fatal(err)
	}
	if o.MissingCount != maxInt64 || o.MissingCount < 0 {
		t.Fatalf("count wrapped: %d", o.MissingCount)
	}
}

func TestEligibleBatchKeysetPagination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strmgen.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	for _, rel := range []string{"A.strm", "B.strm", "C.strm", "D.strm", "E.strm"} {
		if err := s.RecordOwnership(ctx, Ownership{
			SourceKey: "source", RelativePath: rel, AbsolutePath: "/out/" + rel, RemotePath: "/remote/" + rel,
		}, 1); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(
			`UPDATE ownership SET missing_count = 3, missing_round = 1 WHERE source_key = 'source' AND relative_path = ?`,
			rel); err != nil {
			t.Fatal(err)
		}
	}
	after := ""
	var seen []string
	for {
		batch, err := s.ListEligibleAfter(ctx, "source", after, 3, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(batch) == 0 {
			break
		}
		for _, row := range batch {
			seen = append(seen, row.RelativePath)
			after = row.RelativePath
		}
	}
	if len(seen) != 5 || seen[0] != "A.strm" || seen[4] != "E.strm" {
		t.Fatalf("keyset pagination lost or duplicated rows: %v", seen)
	}
}

func TestReadOnlyLegacyDatabaseCannotUseCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "strmgen.db")
	createLegacyDatabase(t, path,
		`INSERT INTO ownership VALUES ('source', 'A.strm', '/out/A.strm', 3, 9);`)
	ro, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("read-only legacy open: %v", err)
	}
	defer ro.Close()
	if ro.PreciseMappingAvailable() {
		t.Fatal("legacy read-only store must not claim a precise mapping")
	}
	o, ok, err := ro.Lookup("source", "A.strm")
	if err != nil || !ok || o.AbsolutePath != "/out/A.strm" {
		t.Fatalf("legacy lookup: %+v ok=%v err=%v", o, ok, err)
	}
	eligible, err := ro.ListEligibleAfter(context.Background(), "source", "", 3, 200)
	if err != nil || len(eligible) != 0 {
		t.Fatalf("legacy read-only candidate: %v %v", eligible, err)
	}
	if _, err := ro.ClearMissing(context.Background(), "source", "A.strm"); err == nil {
		t.Fatal("read-only store must refuse a count change")
	}
	if _, err := ro.CommitRound(context.Background(), "source", 4, time.Now(), nil); err == nil {
		t.Fatal("read-only store must refuse a round commit")
	}
}

func TestReadOnlyRejectsUnknownColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strmgen.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	first.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE ownership ADD COLUMN unexpected TEXT DEFAULT ''`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 3`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := OpenReadOnly(path); err == nil {
		t.Fatal("a newer schema must be rejected")
	}
}

func TestCommitRoundReportsCommittedAdvances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strmgen.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.RecordOwnership(ctx, Ownership{
		SourceKey: "source", RelativePath: "A.strm", AbsolutePath: "/out/A.strm", RemotePath: "/remote/A.mkv",
	}, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordOwnership(ctx, Ownership{
		SourceKey: "source", RelativePath: "B.strm", AbsolutePath: "/out/B.strm", RemotePath: "/remote/B.mkv",
	}, 1); err != nil {
		t.Fatal(err)
	}
	stageMissing(t, ctx, s, "source", "A.strm")
	if err := s.AddPendingMissing(ctx, "source", []PendingMissingEntry{
		{RelativePath: "B.strm", Reason: MissingReasonExcludedExtension},
	}); err != nil {
		t.Fatal(err)
	}
	advances := &recordingAdvances{}
	result, err := s.CommitRound(ctx, "source", 1, time.Unix(0, 0), advances)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Committed {
		t.Fatal("committed round must report the commit fact")
	}
	if len(advances.committed) != 2 {
		t.Fatalf("committed advances: %+v", advances.committed)
	}
	want := map[string]MissingAdvance{
		"A.strm": {RelativePath: "A.strm", PriorCount: 0, NewCount: 1, Reason: MissingReasonSourceTarget},
		"B.strm": {RelativePath: "B.strm", PriorCount: 0, NewCount: 1, Reason: MissingReasonExcludedExtension},
	}
	for _, got := range advances.committed {
		if want[got.RelativePath] != got {
			t.Fatalf("advance mismatch: got %+v want %+v", got, want[got.RelativePath])
		}
	}
	// A repeated commit of the same epoch reports no new advances.
	stageMissing(t, ctx, s, "source", "A.strm")
	advances = &recordingAdvances{}
	result, err = s.CommitRound(ctx, "source", 1, time.Unix(1, 0), advances)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Committed || len(advances.committed) != 0 || len(advances.staged) != 0 {
		t.Fatalf("duplicate epoch: committed=%v advances=%+v", result.Committed, advances)
	}
}

// B review B-01: the advance sink receives records provisionally while the
// transaction runs; a failed or rolled-back commit must discard them so an
// uncommitted candidate can never be published as progress.
func TestCommitRoundDiscardsAdvancesWhenTransactionFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strmgen.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.RecordOwnership(ctx, Ownership{
		SourceKey: "source", RelativePath: "A.strm", AbsolutePath: "/out/A.strm", RemotePath: "/remote/A.mkv",
	}, 1); err != nil {
		t.Fatal(err)
	}
	stageMissing(t, ctx, s, "source", "A.strm")
	// Fail the main transaction after the pending rows are staged.
	if _, err := s.db.Exec(`PRAGMA query_only = true`); err != nil {
		t.Fatal(err)
	}
	advances := &recordingAdvances{}
	if _, err := s.CommitRound(ctx, "source", 1, time.Unix(0, 0), advances); err == nil {
		t.Fatal("a failed main transaction must be reported")
	}
	if len(advances.committed) != 0 || len(advances.staged) != 0 || advances.commits != 0 {
		t.Fatalf("uncommitted candidates were published: %+v", advances)
	}
	if advances.discards == 0 {
		t.Fatal("a failed commit must discard the provisional records")
	}
	o, _, err := s.Lookup("source", "A.strm")
	if err != nil || o.MissingCount != 0 {
		t.Fatalf("a failed commit must not change the count: %+v err=%v", o, err)
	}
}

// B review B-01: with no sink attached, the commit stays correct and collects
// nothing; the durable counts still advance exactly once per epoch.
func TestCommitRoundWithoutSinkKeepsCountSemantics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strmgen.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.RecordOwnership(ctx, Ownership{
		SourceKey: "source", RelativePath: "A.strm", AbsolutePath: "/out/A.strm", RemotePath: "/remote/A.mkv",
	}, 1); err != nil {
		t.Fatal(err)
	}
	stageMissing(t, ctx, s, "source", "A.strm")
	result, err := s.CommitRound(ctx, "source", 1, time.Unix(0, 0), nil)
	if err != nil || !result.Committed {
		t.Fatalf("nil-sink commit: %+v err=%v", result, err)
	}
	if o, _, err := s.Lookup("source", "A.strm"); err != nil || o.MissingCount != 1 {
		t.Fatalf("nil-sink count: %+v err=%v", o, err)
	}
}

func TestCommitRoundReportsCommitWhenScratchCleanupFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strmgen.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.RecordOwnership(ctx, Ownership{
		SourceKey: "source", RelativePath: "A.strm", AbsolutePath: "/out/A.strm", RemotePath: "/remote/A.mkv",
	}, 1); err != nil {
		t.Fatal(err)
	}
	stageMissing(t, ctx, s, "source", "A.strm")
	// Make the private workspace read-only after the pending rows are staged:
	// the staged-batch read still works, the main transaction commits, and
	// only the post-commit scratch cleanup fails.
	if _, err := s.scratch.Exec(`PRAGMA query_only = true`); err != nil {
		t.Fatal(err)
	}
	advances := &recordingAdvances{}
	result, err := s.CommitRound(ctx, "source", 1, time.Unix(0, 0), advances)
	if err == nil {
		t.Fatal("scratch cleanup failure must be reported")
	}
	if !result.Committed {
		t.Fatal("a failed scratch cleanup must not hide the committed round")
	}
	if len(advances.committed) != 1 || advances.committed[0].NewCount != 1 || advances.commits != 1 {
		t.Fatalf("advances: %+v", advances)
	}
	o, ok, err := s.Lookup("source", "A.strm")
	if err != nil || !ok || o.MissingCount != 1 {
		t.Fatalf("durable count: %+v ok=%v err=%v", o, ok, err)
	}
}

func TestClearMissingReportsPriorCommittedCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "strmgen.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.RecordOwnership(ctx, Ownership{
		SourceKey: "source", RelativePath: "A.strm", AbsolutePath: "/out/A.strm", RemotePath: "/remote/A.mkv",
	}, 1); err != nil {
		t.Fatal(err)
	}
	// A row without committed evidence reports the row with a zero count.
	cleared, err := s.ClearMissing(ctx, "source", "A.strm")
	if err != nil || !cleared.RowFound || cleared.PriorMissing != 0 {
		t.Fatalf("clear result: %+v err=%v", cleared, err)
	}
	if _, err := s.db.Exec(
		`UPDATE ownership SET missing_count = 2, missing_round = 1 WHERE source_key = 'source' AND relative_path = 'A.strm'`); err != nil {
		t.Fatal(err)
	}
	cleared, err = s.ClearMissing(ctx, "source", "A.strm")
	if err != nil {
		t.Fatal(err)
	}
	if !cleared.RowFound || cleared.PriorMissing != 2 {
		t.Fatalf("clear result: %+v", cleared)
	}
	o, ok, err := s.Lookup("source", "A.strm")
	if err != nil || !ok || o.MissingCount != 0 || o.MissingRound != 0 {
		t.Fatalf("row after clear: %+v ok=%v err=%v", o, ok, err)
	}
	// A missing row reports not-found.
	cleared, err = s.ClearMissing(ctx, "source", "C.strm")
	if err != nil || cleared.RowFound || cleared.PriorMissing != 0 {
		t.Fatalf("clear result for absent row: %+v err=%v", cleared, err)
	}
}

// B review B-03: when the main-database clear already succeeded and only the
// private scratch cleanup failed, the durable N -> 0 fact must be returned
// together with the error so no caller can miss it.
func TestClearMissingReportsDurableClearAfterScratchFailure(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "review.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.RecordOwnership(ctx, Ownership{SourceKey: "source", RelativePath: "A.strm", AbsolutePath: "/out/A.strm", RemotePath: "/remote/A.mkv"}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE ownership SET missing_count = 2, missing_round = 1`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.scratch.Exec(`PRAGMA query_only = true`); err != nil {
		t.Fatal(err)
	}
	result, err := s.ClearMissing(ctx, "source", "A.strm")
	if err == nil {
		t.Fatal("scratch cleanup failure must be reported")
	}
	if !result.RowFound || result.PriorMissing != 2 {
		t.Fatalf("durable 2 -> 0 fact lost after scratch failure: %+v", result)
	}
	row, ok, err := s.Lookup("source", "A.strm")
	if err != nil || !ok || row.MissingCount != 0 {
		t.Fatalf("durable count after clear: %+v ok=%v err=%v", row, ok, err)
	}
}
