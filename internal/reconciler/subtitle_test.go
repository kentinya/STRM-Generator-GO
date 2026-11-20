package reconciler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"strm-generator-go/internal/openlist"
	"strm-generator-go/internal/pathmap"
	"strm-generator-go/internal/securepath"
	"strm-generator-go/internal/storage"
)

// staticFetcher downloads a fixed byte payload per remote path and records the
// calls, so tests can assert the exact download set of a round.
type staticFetcher struct {
	bodies map[string]string
	calls  []string
	failOn map[string]bool
}

func (f *staticFetcher) Fetch(ctx context.Context, remotePath string, w io.Writer) error {
	f.calls = append(f.calls, remotePath)
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.failOn[remotePath] {
		return errors.New("injected fetch failure")
	}
	body, ok := f.bodies[remotePath]
	if !ok {
		return errors.New("no body for path")
	}
	_, err := w.Write([]byte(body))
	return err
}

func subtitleOptions(fetcher *staticFetcher) Options {
	return Options{
		PublicPrefix:         "https://openlist.example.invalid",
		CleanupEnabled:       false,
		MissingConfirmations: 3,
		SubtitlesEnabled:     true,
		SubtitleFetcher:      fetcher,
	}
}

func subtitleEntry(name string) openlist.Entry {
	size := int64(64)
	return openlist.Entry{Name: name, Size: &size}
}

// subtitleTree has one eligible directory with two videos and two subtitles,
// one subtitle-only directory and a nested eligible directory.
func subtitleTree() *fakeLister {
	return &fakeLister{
		dirs: map[string][]openlist.Entry{
			"/Movies": {
				dir("SubsOnly"),
				dir("科幻"),
				media("Sample.mkv", 10737418240, shaA),
				media("NoHash.mp4", 2048, ""),
				subtitleEntry("Movie.zh.srt"),
				subtitleEntry("中文.srt"),
				subtitleEntry("notes.txt"),
			},
			"/Movies/SubsOnly": {
				subtitleEntry("Orphan.srt"),
			},
			"/Movies/科幻": {
				media("A #100%.mkv", 4096, shaB),
				subtitleEntry("A #100%.zh.srt"),
			},
		},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
}

func subtitleFetcher() *staticFetcher {
	return &staticFetcher{
		bodies: map[string]string{
			"/Movies/Movie.zh.srt":        "1\n00:00:01,000 --> 00:00:02,000\nhello\n",
			"/Movies/中文.srt":              "1\n00:00:01,000 --> 00:00:02,000\n你好\n",
			"/Movies/科幻/A #100%.zh.srt":   "1\n00:00:01,000 --> 00:00:02,000\n科幻\n",
			"/Movies/SubsOnly/Orphan.srt": "never downloaded\n",
		},
		failOn: map[string]bool{},
	}
}

func runSubtitle(t *testing.T, lister *fakeLister, store *storage.Store, root string, opts Options) *Report {
	t.Helper()
	report, err := New(lister, store, opts).Run(context.Background(), []Source{sourceFor(root)})
	if report == nil {
		t.Fatalf("run returned no report: %v", err)
	}
	return report
}

func mustNotFail(t *testing.T, report *Report) SourceResult {
	t.Helper()
	res := report.Sources[0]
	if res.Err != nil || res.Partial || res.Conflicts != 0 || res.Errors != 0 {
		t.Fatalf("round must complete cleanly: %+v", res)
	}
	return res
}

func TestSubtitleDownloadPerEligibleDirectory(t *testing.T) {
	lister := subtitleTree()
	root := t.TempDir()
	store := newStore(t)
	fetcher := subtitleFetcher()

	res := mustNotFail(t, runSubtitle(t, lister, store, root, subtitleOptions(fetcher)))
	if res.SubtitlesDownloaded != 3 {
		t.Fatalf("three subtitles must be published: %+v", res)
	}
	if len(fetcher.calls) != 3 {
		t.Fatalf("fetch calls = %v", fetcher.calls)
	}
	if res.SubtitlesSkipped != 0 || res.SubtitlesFailed != 0 {
		t.Fatalf("unexpected subtitle counters: %+v", res)
	}
	want := map[string]string{
		"Movie.zh.srt":      fetcher.bodies["/Movies/Movie.zh.srt"],
		"中文.srt":            fetcher.bodies["/Movies/中文.srt"],
		"科幻/A #100%.zh.srt": fetcher.bodies["/Movies/科幻/A #100%.zh.srt"],
	}
	for rel, body := range want {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("subtitle %s missing: %v", rel, err)
		}
		if string(raw) != body {
			t.Fatalf("subtitle %s bytes = %q", rel, raw)
		}
		o, ok, err := store.Lookup("/Movies", rel)
		if err != nil || !ok {
			t.Fatalf("subtitle %s ownership: ok=%v err=%v", rel, ok, err)
		}
		if o.FileType != storage.FileSubtitle {
			t.Fatalf("subtitle %s type = %d", rel, o.FileType)
		}
		if o.RemotePath != "/Movies/"+filepath.ToSlash(rel) {
			t.Fatalf("subtitle %s remote = %q", rel, o.RemotePath)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "SubsOnly", "Orphan.srt")); err == nil {
		t.Fatal("a directory without a usable video must not download subtitles")
	}

	// A stable rerun skips every existing file and downloads nothing.
	before := snapshotSubtitles(t, root)
	fetcher2 := subtitleFetcher()
	res = mustNotFail(t, runSubtitle(t, lister, store, root, subtitleOptions(fetcher2)))
	after := snapshotSubtitles(t, root)
	if len(before) != len(after) {
		t.Fatalf("stable rerun changed the file set: %v vs %v", before, after)
	}
	if res.SubtitlesDownloaded != 0 || res.SubtitlesSkipped != 3 {
		t.Fatalf("stable rerun must only skip: %+v", res)
	}
	if len(fetcher2.calls) != 0 {
		t.Fatalf("stable rerun fetched: %v", fetcher2.calls)
	}
	for rel, body := range before {
		if after[rel] != body {
			t.Fatalf("stable rerun changed subtitle %s: %q vs %q", rel, body, after[rel])
		}
	}
}

func TestSubtitleDirectoryWithOnlyConflictingVideosSkips(t *testing.T) {
	lister := subtitleTree()
	// Two videos mapping to one target conflict: no usable result, so the
	// subtitle next to them must not be downloaded.
	lister.dirs["/Movies/科幻"] = []openlist.Entry{
		media("A #100%.mkv", 4096, shaB),
		media("A #100%.mp4", 4096, shaB),
		subtitleEntry("A #100%.zh.srt"),
	}
	root := t.TempDir()
	store := newStore(t)
	fetcher := subtitleFetcher()
	report := runSubtitle(t, lister, store, root, subtitleOptions(fetcher))
	res := report.Sources[0]
	if !res.Partial || res.Conflicts == 0 {
		t.Fatalf("the video conflict must fail the round: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, "科幻", "A #100%.zh.srt")); err == nil {
		t.Fatal("no subtitle may be downloaded for a directory without a usable video")
	}
}

func TestSubtitleExistingPlainFileIsSkippedUnchanged(t *testing.T) {
	lister := subtitleTree()
	root := t.TempDir()
	store := newStore(t)
	// A plain unowned subtitle file exists with foreign bytes and mtime.
	existing := filepath.Join(root, "Movie.zh.srt")
	if err := os.WriteFile(existing, []byte("user-authored subtitle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldInfo, err := os.Stat(existing)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := subtitleFetcher()
	res := mustNotFail(t, runSubtitle(t, lister, store, root, subtitleOptions(fetcher)))
	if res.SubtitlesSkipped != 1 || res.SubtitlesDownloaded != 2 {
		t.Fatalf("existing plain file must be skipped: %+v", res)
	}
	raw, err := os.ReadFile(existing)
	if err != nil || string(raw) != "user-authored subtitle\n" {
		t.Fatalf("existing subtitle bytes changed: %q %v", raw, err)
	}
	newInfo, err := os.Stat(existing)
	if err != nil || !newInfo.ModTime().Equal(oldInfo.ModTime()) {
		t.Fatalf("existing subtitle mtime changed: %v %v", newInfo.ModTime(), oldInfo.ModTime())
	}
	if _, ok, err := store.Lookup("/Movies", "Movie.zh.srt"); err != nil || ok {
		t.Fatalf("an existing unowned file must not be claimed: ok=%v err=%v", ok, err)
	}
}

func TestSubtitleSymlinkTargetIsAConflict(t *testing.T) {
	lister := subtitleTree()
	root := t.TempDir()
	store := newStore(t)
	if err := os.MkdirAll(filepath.Join(root, "科幻"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "科幻", "A #100%.zh.srt")
	if err := os.Symlink("/etc/passwd", link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	fetcher := subtitleFetcher()
	report := runSubtitle(t, lister, store, root, subtitleOptions(fetcher))
	res := report.Sources[0]
	if !res.Partial || res.Conflicts == 0 {
		t.Fatalf("a symlink subtitle target must conflict: %+v", res)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink was not preserved: %v", err)
	}
}

func TestSubtitleDownloadFailureFreezesTheRound(t *testing.T) {
	lister := subtitleTree()
	root := t.TempDir()
	store := newStore(t)
	fetcher := subtitleFetcher()
	fetcher.failOn["/Movies/中文.srt"] = true
	report := runSubtitle(t, lister, store, root, subtitleOptions(fetcher))
	res := report.Sources[0]
	if !res.Partial || res.Errors == 0 || res.SubtitlesFailed != 1 {
		t.Fatalf("the failed download must fail the round: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, "中文.srt")); err == nil {
		t.Fatal("a failed download must not leave a target")
	}
	if res.SubtitlesDownloaded != 2 {
		t.Fatalf("earlier publications keep standing: %+v", res)
	}
	// No round was committed, so the epoch stays at one.
	if epoch, err := store.NextEpoch("/Movies"); err != nil || epoch != 1 {
		t.Fatalf("an incomplete round must not commit: epoch=%d err=%v", epoch, err)
	}
	// The recovery rerun downloads the missing subtitle.
	fetcher.failOn["/Movies/中文.srt"] = false
	res = mustNotFail(t, runSubtitle(t, lister, store, root, subtitleOptions(fetcher)))
	if res.SubtitlesDownloaded != 1 || res.SubtitlesSkipped != 2 {
		t.Fatalf("recovery rerun: %+v", res)
	}
}

func TestSubtitleDryRunPlansWithoutSideEffects(t *testing.T) {
	lister := subtitleTree()
	root := t.TempDir()
	store := newStore(t)
	opts := subtitleOptions(subtitleFetcher())
	opts.DryRun = true
	report := runSubtitle(t, lister, store, root, opts)
	res := report.Sources[0]
	if res.SubtitlesDownloaded != 3 {
		t.Fatalf("dry-run must plan the three downloads: %+v", res)
	}
	if res.Partial {
		t.Fatalf("dry-run must stay clean: %+v", res)
	}
	for rel := range map[string]bool{"Movie.zh.srt": true, "中文.srt": true, "科幻/A #100%.zh.srt": true} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			t.Fatalf("dry-run created %s", rel)
		}
	}
	if _, ok, err := store.Lookup("/Movies", "中文.srt"); err != nil || ok {
		t.Fatalf("dry-run must not register ownership: ok=%v err=%v", ok, err)
	}
	if epoch, err := store.NextEpoch("/Movies"); err != nil || epoch != 1 {
		t.Fatalf("dry-run must not commit a round: epoch=%d err=%v", epoch, err)
	}
	if len(snapshotSubtitles(t, root)) != 0 {
		t.Fatal("dry-run left files behind")
	}
}

func TestSubtitleThreeRoundsThenDeleteAndReappearClears(t *testing.T) {
	lister := subtitleTree()
	root := t.TempDir()
	store := newStore(t)
	opts := subtitleOptions(subtitleFetcher())
	opts.CleanupEnabled = true
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))

	// The subtitle disappears from the listing and from Get; the video stays.
	removeRemote(lister, "中文.srt")
	for round := 1; round <= 3; round++ {
		res := mustNotFail(t, runSubtitle(t, lister, store, root, opts))
		if res.SubtitlesDeleted != 0 {
			t.Fatalf("round %d deleted too early: %+v", round, res)
		}
		o, ok, err := store.Lookup("/Movies", "中文.srt")
		if err != nil || !ok || o.MissingCount != int64(round) {
			t.Fatalf("round %d count: %+v ok=%v err=%v", round, o, ok, err)
		}
	}
	res := mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	if res.SubtitlesDeleted != 1 {
		t.Fatalf("the fourth round must delete the owned subtitle: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, "中文.srt")); !os.IsNotExist(err) {
		t.Fatal("the owned subtitle was not removed")
	}
	if _, ok, _ := store.Lookup("/Movies", "中文.srt"); ok {
		t.Fatal("the deleted subtitle kept its ownership row")
	}
	if _, err := os.Stat(filepath.Join(root, "Sample.strm")); err != nil {
		t.Fatal("the unrelated STRM was removed")
	}

	// A second subtitle reappears after two missing rounds and must clear.
	removeRemote(lister, "Movie.zh.srt")
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	if o, ok, _ := store.Lookup("/Movies", "Movie.zh.srt"); !ok || o.MissingCount != 2 {
		t.Fatalf("two missing rounds expected: %+v ok=%v", o, ok)
	}
	delete(lister.getErr, "/Movies/Movie.zh.srt")
	lister.dirs["/Movies"] = append(lister.dirs["/Movies"], subtitleEntry("Movie.zh.srt"))
	res = mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	if o, ok, _ := store.Lookup("/Movies", "Movie.zh.srt"); !ok || o.MissingCount != 0 {
		t.Fatalf("a reappearance must clear the count: %+v ok=%v", o, ok)
	}
	if res.SubtitlesDeleted != 0 {
		t.Fatalf("the reappearance must not delete: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, "Movie.zh.srt")); err != nil {
		t.Fatal("the reappearing subtitle must stay on disk")
	}
}

func TestSubtitleExcludedExtensionCountsAndDeletes(t *testing.T) {
	lister := subtitleTree()
	root := t.TempDir()
	store := newStore(t)
	opts := subtitleOptions(subtitleFetcher())
	opts.CleanupEnabled = true
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))

	// The subtitle extension list narrows to .vtt only: the owned .srt files
	// keep reading successfully on Get but count as excluded outputs.
	opts.SubtitleExtensions = pathmap.NewMediaSet([]string{".vtt"})
	lister.getFile = map[string]bool{
		"/Movies/Movie.zh.srt":      true,
		"/Movies/中文.srt":            true,
		"/Movies/科幻/A #100%.zh.srt": true,
	}
	for round := 1; round <= 3; round++ {
		res := mustNotFail(t, runSubtitle(t, lister, store, root, opts))
		if res.SubtitlesDeleted != 0 {
			t.Fatalf("round %d deleted too early: %+v", round, res)
		}
	}
	res := mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	if res.SubtitlesDeleted != 3 {
		t.Fatalf("the excluded subtitles must be deleted at the threshold: %+v", res)
	}
	for _, rel := range []string{"Movie.zh.srt", "中文.srt", "科幻/A #100%.zh.srt"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("%s was not removed", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "Sample.strm")); err != nil {
		t.Fatal("the video STRM must stay")
	}
}

func TestSubtitlesDisabledFreezesSubtitleRowsAndCounts(t *testing.T) {
	lister := subtitleTree()
	root := t.TempDir()
	store := newStore(t)
	opts := subtitleOptions(subtitleFetcher())
	opts.CleanupEnabled = true
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	removeRemote(lister, "中文.srt")
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	if o, ok, _ := store.Lookup("/Movies", "中文.srt"); !ok || o.MissingCount != 1 {
		t.Fatal("the seed round must have advanced the count once")
	}

	// Closing the subtitle feature freezes the row: no further counting, no
	// deletion, the file itself stays untouched.
	opts.SubtitlesEnabled = false
	for round := 0; round < 5; round++ {
		res := mustNotFail(t, runSubtitle(t, lister, store, root, opts))
		if res.SubtitlesDeleted != 0 {
			t.Fatalf("a closed feature must never delete: %+v", res)
		}
	}
	if o, ok, _ := store.Lookup("/Movies", "中文.srt"); !ok || o.MissingCount != 1 {
		t.Fatalf("the count must stay frozen: %+v ok=%v", o, ok)
	}
	if _, err := os.Stat(filepath.Join(root, "中文.srt")); err != nil {
		t.Fatal("the owned subtitle file must stay on disk")
	}
	// The STRM pipeline keeps working while subtitles are closed.
	if res := mustNotFail(t, runSubtitle(t, lister, store, root, opts)); res.SubtitlesDownloaded != 0 {
		t.Fatalf("a closed feature must not download: %+v", res)
	}
}

func TestSubtitlePresenceClearsWithCleanupDisabled(t *testing.T) {
	lister := subtitleTree()
	root := t.TempDir()
	store := newStore(t)
	opts := subtitleOptions(subtitleFetcher())
	opts.CleanupEnabled = true
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	removeRemote(lister, "中文.srt")
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	if o, ok, _ := store.Lookup("/Movies", "中文.srt"); !ok || o.MissingCount != 1 {
		t.Fatalf("seed count expected: %+v ok=%v", o, ok)
	}

	// Cleanup is disabled and the listing no longer carries the subtitle, but
	// the exact Get still reads it (a list/get cache inconsistency). The
	// appearance path must clear the committed count without any deletion.
	opts.CleanupEnabled = false
	delete(lister.getErr, "/Movies/中文.srt")
	lister.getFile = map[string]bool{"/Movies/中文.srt": true}
	res := mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	if o, ok, _ := store.Lookup("/Movies", "中文.srt"); !ok || o.MissingCount != 0 {
		t.Fatalf("the confirmed reappearance must clear: %+v ok=%v", o, ok)
	}
	if res.SubtitlesDeleted != 0 {
		t.Fatalf("nothing may be deleted with cleanup disabled: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, "中文.srt")); err != nil {
		t.Fatal("the subtitle file must stay")
	}
}

func TestSubtitleCleanupReclaimsRowWhenLocalFileAlreadyGone(t *testing.T) {
	lister := subtitleTree()
	root := t.TempDir()
	store := newStore(t)
	opts := subtitleOptions(subtitleFetcher())
	opts.CleanupEnabled = true
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	removeRemote(lister, "中文.srt")
	for round := 1; round <= 3; round++ {
		mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	}
	// The local file vanishes before the deletion round: the row is reclaimed
	// instead of counted as an unlink.
	if err := os.Remove(filepath.Join(root, "中文.srt")); err != nil {
		t.Fatal(err)
	}
	res := mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	if res.SubtitlesDeleted != 0 || res.SubtitlesReclaimed != 1 {
		t.Fatalf("an already absent subtitle must be reclaimed: %+v", res)
	}
	if _, ok, _ := store.Lookup("/Movies", "中文.srt"); ok {
		t.Fatal("the reclaimed row must be gone")
	}
}

func snapshotSubtitles(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.HasSuffix(path, ".strm") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		out[filepath.ToSlash(rel)+"|"+fmt.Sprint(info.ModTime().UnixNano())] = string(raw)
		return nil
	})
	return out
}

func TestSubtitleDownloadFailureFreezesThresholdCleanup(t *testing.T) {
	lister := subtitleTree()
	root := t.TempDir()
	store := newStore(t)
	opts := subtitleOptions(subtitleFetcher())
	opts.CleanupEnabled = true
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	// One subtitle reaches the committed threshold; a new download in the same
	// round then fails and must freeze both the deletion and the round.
	removeRemote(lister, "中文.srt")
	for round := 1; round <= 3; round++ {
		mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	}
	lister.dirs["/Movies"] = append(lister.dirs["/Movies"], subtitleEntry("New.srt"))
	failing := subtitleFetcher()
	failing.bodies["/Movies/New.srt"] = "new subtitle\n"
	failing.failOn["/Movies/New.srt"] = true
	report := runSubtitle(t, lister, store, root, Options{
		PublicPrefix:         "https://openlist.example.invalid",
		CleanupEnabled:       true,
		MissingConfirmations: 3,
		SubtitlesEnabled:     true,
		SubtitleFetcher:      failing,
	})
	res := report.Sources[0]
	if !res.Partial || res.SubtitlesDeleted != 0 {
		t.Fatalf("a failing download round must not clean up: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, "中文.srt")); err != nil {
		t.Fatal("the threshold subtitle must stay on disk through the failed round")
	}
	// The recovery round downloads the missing file and deletes the qualified
	// subtitle in the same pass.
	delete(failing.failOn, "/Movies/New.srt")
	res = mustNotFail(t, runSubtitle(t, lister, store, root, Options{
		PublicPrefix:         "https://openlist.example.invalid",
		CleanupEnabled:       true,
		MissingConfirmations: 3,
		SubtitlesEnabled:     true,
		SubtitleFetcher:      failing,
	}))
	if res.SubtitlesDownloaded != 1 || res.SubtitlesDeleted != 1 {
		t.Fatalf("recovery round must download and delete: %+v", res)
	}
}

func TestSubtitleDryRunPreviewsCleanupCandidatesWithoutDeletes(t *testing.T) {
	lister := subtitleTree()
	root := t.TempDir()
	store := newStore(t)
	opts := subtitleOptions(subtitleFetcher())
	opts.CleanupEnabled = true
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	removeRemote(lister, "中文.srt")
	for round := 1; round <= 3; round++ {
		mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	}
	lister.getFile = map[string]bool{"/Movies/中文.srt": false}
	dry := opts
	dry.DryRun = true
	res := mustNotFail(t, runSubtitle(t, lister, store, root, dry))
	if res.SubtitlesCandidates != 1 || res.SubtitlesDeleted != 0 {
		t.Fatalf("dry-run must preview exactly the qualified subtitle: %+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, "中文.srt")); err != nil {
		t.Fatal("dry-run must not delete")
	}
	if o, ok, _ := store.Lookup("/Movies", "中文.srt"); !ok || o.MissingCount != 3 {
		t.Fatalf("dry-run must not change counts: %+v ok=%v", o, ok)
	}
}

func TestSubtitleLifecycleNeverEntersTheFactStream(t *testing.T) {
	lister := subtitleTree()
	root := t.TempDir()
	store := newStore(t)
	opts := subtitleOptions(subtitleFetcher())
	opts.CleanupEnabled = true
	sink := &recordingSink{}
	opts.Facts = sink
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	// The seed round publishes three STRM files and three subtitles; only the
	// STRM publications may appear as facts.
	if len(sink.facts) != 3 {
		t.Fatalf("subtitle publications must not emit STRM facts: %+v", sink.facts)
	}
	for _, fact := range sink.facts {
		if fact.Kind != FactCreated || !strings.HasSuffix(fact.RelativePath, ".strm") {
			t.Fatalf("unexpected subtitle fact: %+v", fact)
		}
	}

	// A committed subtitle missing advance and a subtitle deletion stay in the
	// command statistics; the fact stream keeps carrying STRM events only.
	removeRemote(lister, "中文.srt")
	seeded := mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	if seeded.ConfirmedMissing != 1 {
		t.Fatalf("the subtitle count must still be staged: %+v", seeded)
	}
	sink.facts = nil
	if len(sink.facts) != 0 {
		t.Fatalf("a subtitle advance reached the fact stream: %+v", sink.facts)
	}
	for round := 0; round < 2; round++ {
		mustNotFail(t, runSubtitle(t, lister, store, root, opts))
		sink.facts = nil
	}
	final := mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	if final.SubtitlesDeleted != 1 {
		t.Fatalf("the subtitle must be deleted by its own evidence: %+v", final)
	}
	if len(sink.facts) != 0 {
		t.Fatalf("a subtitle deletion reached the fact stream: %+v", sink.facts)
	}
	// The STRM pipeline in the same rounds keeps producing ordinary facts.
	sink.facts = nil
	restore := append([]openlist.Entry(nil), lister.dirs["/Movies"]...)
	lister.dirs["/Movies"] = restore
	removeRemote(lister, "Sample.mkv")
	missing := mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	if missing.ConfirmedMissing != 1 {
		t.Fatalf("the STRM count must be staged beside the subtitle pipeline: %+v", missing)
	}
	advances := factKindsOf(sink, FactMissingAdvanced)
	if len(advances) != 1 || advances[0].RelativePath != "Sample.strm" {
		t.Fatalf("STRM advances must still be reported: %+v", advances)
	}
}

// ownershipFailureStore rejects the ownership insert of selected relative
// paths, mirroring a state database that refuses the video's row.
type ownershipFailureStore struct {
	*storage.Store
	failOn func(relativePath string) bool
}

func (f *ownershipFailureStore) RecordOwnership(ctx context.Context, o storage.Ownership, epoch int64) error {
	if f.failOn(o.RelativePath) {
		return errors.New("injected ownership failure")
	}
	return f.Store.RecordOwnership(ctx, o, epoch)
}

// A directory whose only video never completed its ownership record must not
// fetch the subtitle beside it: an unowned STRM cannot qualify the directory.
func TestSubtitleDirectoryWaitsForCompletedVideoOwnership(t *testing.T) {
	lister := &fakeLister{
		dirs: map[string][]openlist.Entry{
			"/Movies": {media("Sample.mkv", 10, shaA), subtitleEntry("Movie.zh.srt")},
		},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
	root := t.TempDir()
	store := newStore(t)
	fetcher := subtitleFetcher()
	report, err := New(lister,
		&ownershipFailureStore{Store: store, failOn: func(string) bool { return true }},
		subtitleOptions(fetcher)).Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("the ownership failure must fail the source")
	}
	res := report.Sources[0]
	if !res.Partial || res.Errors == 0 {
		t.Fatalf("ownership failure must be reported: %+v", res)
	}
	if len(fetcher.calls) != 0 {
		t.Fatalf("subtitle content was requested without a usable video: %v", fetcher.calls)
	}
	if _, statErr := os.Stat(filepath.Join(root, "Movie.zh.srt")); !os.IsNotExist(statErr) {
		t.Fatal("a subtitle was published for an unowned video")
	}
	if _, ok, _ := store.Lookup("/Movies", "Movie.zh.srt"); ok {
		t.Fatal("a subtitle row was recorded for an unowned video")
	}
}

// The same directory keeps its normal subtitle behaviour as soon as one video
// completes its ownership record.
func TestSubtitleDirectoryDownloadsBesideTheCompletingVideo(t *testing.T) {
	lister := &fakeLister{
		dirs: map[string][]openlist.Entry{
			"/Movies": {
				media("Sample.mkv", 10, shaA),
				media("Other.mkv", 20, shaB),
				subtitleEntry("Movie.zh.srt"),
			},
		},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
	root := t.TempDir()
	store := newStore(t)
	fetcher := subtitleFetcher()
	report, err := New(lister,
		&ownershipFailureStore{Store: store, failOn: func(rel string) bool { return rel == "Sample.strm" }},
		subtitleOptions(fetcher)).Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("the ownership failure must fail the source")
	}
	res := report.Sources[0]
	if !res.Partial || res.SubtitlesDownloaded != 1 {
		t.Fatalf("round result: %+v", res)
	}
	if len(fetcher.calls) != 1 || fetcher.calls[0] != "/Movies/Movie.zh.srt" {
		t.Fatalf("download set = %v, want the subtitle beside the completing video", fetcher.calls)
	}
	raw, statErr := os.ReadFile(filepath.Join(root, "Movie.zh.srt"))
	if statErr != nil {
		t.Fatalf("the subtitle was not published: %v", statErr)
	}
	if string(raw) != fetcher.bodies["/Movies/Movie.zh.srt"] {
		t.Fatalf("published bytes = %q", raw)
	}
	o, ok, lookupErr := store.Lookup("/Movies", "Movie.zh.srt")
	if lookupErr != nil || !ok || o.FileType != storage.FileSubtitle {
		t.Fatalf("subtitle ownership: ok=%v err=%v row=%+v", ok, lookupErr, o)
	}
	if _, ok, _ := store.Lookup("/Movies", "Sample.strm"); ok {
		t.Fatal("the unrecorded video must stay unowned")
	}
}

// A committed subtitle reappearance clears its count without entering the
// STRM fact stream, while the video reappearance in the same round still does.
func TestSubtitleReappearanceClearNeverEntersTheFactStream(t *testing.T) {
	sample := media("Sample.mkv", 10, shaA)
	subtitle := subtitleEntry("Movie.zh.srt")
	lister := &fakeLister{
		dirs: map[string][]openlist.Entry{
			"/Movies": {sample, subtitle},
		},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
	root := t.TempDir()
	store := newStore(t)
	opts := subtitleOptions(subtitleFetcher())
	opts.CleanupEnabled = true
	sink := &recordingSink{}
	opts.Facts = sink
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))

	// Both targets disappear: one committed count each.
	removeRemote(lister, "Sample.mkv")
	removeRemote(lister, "Movie.zh.srt")
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	if got := countFor(t, store, "Sample.strm"); got != 1 {
		t.Fatalf("Sample.strm count = %d, want 1", got)
	}
	if got := countFor(t, store, "Movie.zh.srt"); got != 1 {
		t.Fatalf("Movie.zh.srt count = %d, want 1", got)
	}

	// Both reappear in one round: only the STRM clear may reach the fact
	// stream, and both committed counts drop back to zero.
	lister.dirs["/Movies"] = append(lister.dirs["/Movies"], sample, subtitle)
	delete(lister.getErr, "/Movies/Sample.mkv")
	delete(lister.getErr, "/Movies/Movie.zh.srt")
	sink.facts = nil
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	clears := factKindsOf(sink, FactMissingCleared)
	if len(clears) != 1 || clears[0].RelativePath != "Sample.strm" {
		t.Fatalf("cleared facts = %+v, want only the STRM reappearance", clears)
	}
	if got := countFor(t, store, "Movie.zh.srt"); got != 0 {
		t.Fatalf("subtitle count after reappearance = %d, want 0", got)
	}
	if got := countFor(t, store, "Sample.strm"); got != 0 {
		t.Fatalf("STRM count after reappearance = %d, want 0", got)
	}
}

// One failing round freezes both kinds of threshold evidence: the STRM and the
// subtitle that already reached the committed threshold stay on disk with
// their counts untouched, and the recovery round then completes both.
func TestFailedRoundFreezesThresholdSTRMAndSubtitle(t *testing.T) {
	lister := &fakeLister{
		dirs: map[string][]openlist.Entry{
			"/Movies": {
				media("Sample.mkv", 10, shaA),
				media("Other.mkv", 20, shaB),
				subtitleEntry("Movie.zh.srt"),
			},
		},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
	root := t.TempDir()
	store := newStore(t)
	opts := subtitleOptions(subtitleFetcher())
	opts.CleanupEnabled = true
	mustNotFail(t, runSubtitle(t, lister, store, root, opts))

	removeRemote(lister, "Other.mkv")
	removeRemote(lister, "Movie.zh.srt")
	for round := 0; round < 3; round++ {
		mustNotFail(t, runSubtitle(t, lister, store, root, opts))
	}
	if got := countFor(t, store, "Other.strm"); got != 3 {
		t.Fatalf("Other.strm count = %d, want 3", got)
	}
	if got := countFor(t, store, "Movie.zh.srt"); got != 3 {
		t.Fatalf("Movie.zh.srt count = %d, want 3", got)
	}

	// A new subtitle download fails in the same round as both threshold rows.
	lister.dirs["/Movies"] = append(lister.dirs["/Movies"], subtitleEntry("New.srt"))
	failing := subtitleFetcher()
	failing.bodies["/Movies/New.srt"] = "new subtitle\n"
	failing.failOn["/Movies/New.srt"] = true
	failingOpts := opts
	failingOpts.SubtitleFetcher = failing
	frozen := runSubtitle(t, lister, store, root, failingOpts).Sources[0]
	if !frozen.Partial || frozen.Deleted != 0 || frozen.SubtitlesDeleted != 0 {
		t.Fatalf("a failing round must freeze both cleanups: %+v", frozen)
	}
	if frozen.SubtitlesFailed != 1 {
		t.Fatalf("subtitle failures = %d, want 1: %+v", frozen.SubtitlesFailed, frozen)
	}
	for _, rel := range []string{"Other.strm", "Movie.zh.srt"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s must stay on disk through the failed round: %v", rel, err)
		}
	}
	if got := countFor(t, store, "Other.strm"); got != 3 {
		t.Fatalf("failed round changed the STRM count to %d", got)
	}
	if got := countFor(t, store, "Movie.zh.srt"); got != 3 {
		t.Fatalf("failed round changed the subtitle count to %d", got)
	}

	// The recovery round downloads the missing subtitle and deletes both
	// qualified rows in the same pass.
	delete(failing.failOn, "/Movies/New.srt")
	recovered := mustNotFail(t, runSubtitle(t, lister, store, root, failingOpts))
	if recovered.SubtitlesDownloaded != 1 || recovered.SubtitlesDeleted != 1 || recovered.Deleted != 1 {
		t.Fatalf("recovery round counters: %+v", recovered)
	}
	for _, rel := range []string{"Other.strm", "Movie.zh.srt"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Fatalf("%s must be removed by the recovery round: %v", rel, err)
		}
	}
}

// An update whose ownership refresh was refused keeps the update fact but
// never qualifies the directory for subtitle downloads.
func TestSubtitleDirectoryWaitsForOwnershipRefreshOnUpdate(t *testing.T) {
	sample := media("Sample.mkv", 10, shaA)
	lister := &fakeLister{
		dirs: map[string][]openlist.Entry{
			"/Movies": {sample, subtitleEntry("Movie.zh.srt")},
		},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
	root := t.TempDir()
	store := newStore(t)
	fetcher := subtitleFetcher()
	mustNotFail(t, runSubtitle(t, lister, store, root, subtitleOptions(fetcher)))

	// The remote content changes: the update completes on disk while the
	// ownership refresh is refused, so the directory loses its eligibility.
	lister.dirs["/Movies"] = []openlist.Entry{
		media("Sample.mkv", 10, shaB),
		subtitleEntry("Movie.zh.srt"),
		subtitleEntry("Extra.srt"),
	}
	next := subtitleFetcher()
	next.bodies["/Movies/Extra.srt"] = "extra\n"
	report, err := New(lister,
		&ownershipFailureStore{Store: store, failOn: func(rel string) bool { return rel == "Sample.strm" }},
		subtitleOptions(next)).Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("the ownership refresh failure must fail the source")
	}
	res := report.Sources[0]
	if res.Updated != 1 || res.Errors == 0 {
		t.Fatalf("update result: %+v", res)
	}
	if len(next.calls) != 0 {
		t.Fatalf("subtitle content was requested without a refreshed owner: %v", next.calls)
	}
	if _, statErr := os.Stat(filepath.Join(root, "Extra.srt")); !os.IsNotExist(statErr) {
		t.Fatal("a subtitle was published beside an unrefreshed video")
	}
}

// A creation whose directory durability step failed keeps the created fact but
// never qualifies the directory for subtitle downloads.
func TestSubtitleDirectoryWaitsForDurablePublication(t *testing.T) {
	lister := &fakeLister{
		dirs: map[string][]openlist.Entry{
			"/Movies": {media("Sample.mkv", 10, shaA), subtitleEntry("Movie.zh.srt")},
		},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
	root := t.TempDir()
	store := newStore(t)
	fetcher := subtitleFetcher()
	opts := subtitleOptions(fetcher)
	opts.FS = LocalFS{syncDir: func(_ *securepath.Dir) error {
		return errors.New("post-link directory sync failure")
	}}
	report, err := New(lister, store, opts).Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("the durability failure must fail the source")
	}
	res := report.Sources[0]
	if res.Created != 1 || res.Errors == 0 {
		t.Fatalf("publication result: %+v", res)
	}
	if len(fetcher.calls) != 0 {
		t.Fatalf("subtitle content was requested without a durable publication: %v", fetcher.calls)
	}
	if _, statErr := os.Stat(filepath.Join(root, "Movie.zh.srt")); !os.IsNotExist(statErr) {
		t.Fatal("a subtitle was published beside an undurable video")
	}
}
