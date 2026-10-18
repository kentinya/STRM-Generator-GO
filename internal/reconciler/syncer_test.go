package reconciler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"strm-generator-go/internal/openlist"
	"strm-generator-go/internal/storage"
)

// fakeLister is a programmable remote tree.
type fakeLister struct {
	dirs     map[string][]openlist.Entry
	getErr   map[string]error
	listErr  map[string]error
	getCalls int
	rootFile bool
}

func (f *fakeLister) Get(_ context.Context, path string) (*openlist.Entry, error) {
	f.getCalls++
	if err := f.getErr[path]; err != nil {
		return nil, err
	}
	if f.rootFile && path == "/Movies" {
		return &openlist.Entry{Name: "Movies", IsDir: false}, nil
	}
	return &openlist.Entry{Name: filepath.Base(path), IsDir: true}, nil
}

func (f *fakeLister) List(_ context.Context, path string) ([]openlist.Entry, error) {
	if err := f.listErr[path]; err != nil {
		return nil, err
	}
	entries, ok := f.dirs[path]
	if !ok {
		return nil, fmt.Errorf("not found: %s", path)
	}
	return entries, nil
}

func media(name string, size int64, sha string) openlist.Entry {
	e := openlist.Entry{Name: name, Size: &size}
	if sha != "" {
		e.SHA1 = sha
		e.HasSHA = true
	}
	return e
}

func dir(name string) openlist.Entry { return openlist.Entry{Name: name, IsDir: true} }

func newStore(t *testing.T) *storage.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data", "strmgen.db")
	s, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

const (
	shaA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	shaB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func standardTree() *fakeLister {
	return &fakeLister{
		dirs: map[string][]openlist.Entry{
			"/Movies": {
				dir("科幻"),
				media("Sample.mkv", 10737418240, shaA),
				media("NoHash.mp4", 2048, ""),
				{Name: "notes.txt", Size: int64Ptr(8)},
			},
			"/Movies/科幻": {
				media("A #100%.mkv", 4096, shaB),
				media("InvalidHash.mkv", 0, ""),
			},
		},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
}

func int64Ptr(v int64) *int64 { return &v }

func sourceFor(root string) Source {
	return Source{Index: 0, Key: "/Movies", Remote: "/Movies", Output: root}
}

func TestFirstRunCreatesAndCommits(t *testing.T) {
	root := t.TempDir()
	store := newStore(t)
	s := New(standardTree(), store, Options{PublicPrefix: "https://openlist.example.invalid"})
	report, err := s.Run(context.Background(), []Source{sourceFor(root)})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	res := report.Sources[0]
	if res.Created != 4 || res.Skipped != 0 || res.Conflicts != 0 || res.Filtered != 1 || res.Errors != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
	files := collectFiles(t, root)
	if len(files) != 4 {
		t.Fatalf("expected 4 strm files, got %v", files)
	}
	epoch, err := store.NextEpoch("/Movies")
	if err != nil || epoch != 2 {
		t.Fatalf("round not committed: epoch=%d err=%v", epoch, err)
	}
	for _, rel := range []string{"Sample.strm", "NoHash.strm", "科幻/A #100%.strm", "科幻/InvalidHash.strm"} {
		if _, ok, err := store.Lookup("/Movies", rel); err != nil || !ok {
			t.Fatalf("missing ownership for %s: %v %v", rel, ok, err)
		}
	}
	// The generated Sample file must not contain the fixture signature URL.
	raw, _ := os.ReadFile(filepath.Join(root, "Sample.strm"))
	if strings.Contains(string(raw), "sign=") || strings.Contains(string(raw), "raw_url") {
		t.Fatalf("unsafe URL leaked: %q", raw)
	}
	if !strings.Contains(string(raw), "https://openlist.example.invalid/d/Movies/Sample.mkv") {
		t.Fatalf("unexpected URL: %q", raw)
	}
}

func TestRepeatRunSkipsAndPreservesMtime(t *testing.T) {
	root := t.TempDir()
	store := newStore(t)
	opts := Options{PublicPrefix: "https://openlist.example.invalid"}
	s := New(standardTree(), store, opts)
	if _, err := s.Run(context.Background(), []Source{sourceFor(root)}); err != nil {
		t.Fatal(err)
	}
	before := statTimes(t, root)
	s2 := New(standardTree(), store, opts)
	report, err := s2.Run(context.Background(), []Source{sourceFor(root)})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if report.Sources[0].Created != 0 || report.Sources[0].Skipped != 4 {
		t.Fatalf("second run result: %+v", report.Sources[0])
	}
	after := statTimes(t, root)
	for name, mod := range before {
		if !after[name].Equal(mod) {
			t.Fatalf("mtime changed for %s", name)
		}
	}
}

func TestUnownedExistingFileIsConflict(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "科幻"), 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(root, "Sample.strm")
	if err := os.WriteFile(sentinel, []byte("pre-existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := newStore(t)
	s := New(standardTree(), store, Options{PublicPrefix: "https://openlist.example.invalid"})
	report, err := s.Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("conflict must produce an error")
	}
	if report.Sources[0].Conflicts != 1 {
		t.Fatalf("expected one conflict: %+v", report.Sources[0])
	}
	raw, _ := os.ReadFile(sentinel)
	if string(raw) != "pre-existing" {
		t.Fatalf("unowned file was modified: %q", raw)
	}
	if _, ok, _ := store.Lookup("/Movies", "Sample.strm"); ok {
		t.Fatal("unowned file must not be claimed")
	}
}

func TestSymlinkTargetIsConflict(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "sentinel")
	if err := os.WriteFile(target, []byte("sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "Sample.strm")); err != nil {
		t.Fatal(err)
	}
	store := newStore(t)
	s := New(standardTree(), store, Options{PublicPrefix: "https://openlist.example.invalid"})
	report, err := s.Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("symlink conflict must error")
	}
	if report.Sources[0].Conflicts == 0 {
		t.Fatalf("expected symlink conflict: %+v", report.Sources[0])
	}
	raw, _ := os.ReadFile(target)
	if string(raw) != "sentinel" {
		t.Fatalf("sentinel modified: %q", raw)
	}
}

func TestSameTargetConflictAcrossPages(t *testing.T) {
	lister := &fakeLister{
		dirs: map[string][]openlist.Entry{
			"/Movies": {
				media("A.mkv", 10, shaA),
				media("A.mp4", 20, shaB),
			},
		},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
	root := t.TempDir()
	store := newStore(t)
	s := New(lister, store, Options{PublicPrefix: "https://openlist.example.invalid"})
	report, err := s.Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("same-target conflict must error")
	}
	if report.Sources[0].Conflicts != 1 || report.Sources[0].Created != 0 {
		t.Fatalf("conflict result: %+v", report.Sources[0])
	}
	if _, statErr := os.Stat(filepath.Join(root, "A.strm")); !os.IsNotExist(statErr) {
		t.Fatal("conflicted target must not be created")
	}
	if _, ok, _ := store.Lookup("/Movies", "A.strm"); ok {
		t.Fatal("conflicted target must not be owned")
	}
}

func TestNeedsUpdateIsNotSilentlySkipped(t *testing.T) {
	root := t.TempDir()
	store := newStore(t)
	opts := Options{PublicPrefix: "https://openlist.example.invalid"}
	s := New(standardTree(), store, opts)
	if _, err := s.Run(context.Background(), []Source{sourceFor(root)}); err != nil {
		t.Fatal(err)
	}
	// Change the URL for Sample.mkv by using a different public prefix.
	changed := standardTree()
	changed.dirs["/Movies"][1] = media("Sample.mkv", 10737418240, shaB) // hash changed
	s2 := New(changed, store, opts)
	report, err := s2.Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("needs-update must produce a non-zero result")
	}
	if report.Sources[0].NeedsUpdate != 1 {
		t.Fatalf("expected needs_update: %+v", report.Sources[0])
	}
	raw, _ := os.ReadFile(filepath.Join(root, "Sample.strm"))
	if !strings.Contains(string(raw), shaA) {
		t.Fatalf("old file must be preserved: %q", raw)
	}
	// Round must not advance on a needs-update result.
	if epoch, _ := store.NextEpoch("/Movies"); epoch != 2 {
		t.Fatalf("round advanced despite pending update: %d", epoch)
	}
}

func TestSameSHADifferentSizeIsAnomalyNotRewrite(t *testing.T) {
	root := t.TempDir()
	store := newStore(t)
	opts := Options{PublicPrefix: "https://openlist.example.invalid"}
	s := New(standardTree(), store, opts)
	if _, err := s.Run(context.Background(), []Source{sourceFor(root)}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(root, "Sample.strm"))
	changed := standardTree()
	changed.dirs["/Movies"][1] = media("Sample.mkv", 999, shaA)
	s2 := New(changed, store, opts)
	report, err := s2.Run(context.Background(), []Source{sourceFor(root)})
	if err != nil {
		t.Fatalf("same-sha anomaly should not fail: %v", err)
	}
	if report.Sources[0].Skipped != 4 {
		t.Fatalf("expected skip: %+v", report.Sources[0])
	}
	after, _ := os.ReadFile(filepath.Join(root, "Sample.strm"))
	if string(before) != string(after) {
		t.Fatal("file must not be rewritten for a same-hash size anomaly")
	}
	found := false
	for _, d := range report.Sources[0].Diagnostics {
		if strings.Contains(d, "size differs") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected size anomaly diagnostic: %v", report.Sources[0].Diagnostics)
	}
}

func TestDryRunHasNoSideEffects(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "data", "strmgen.db")
	store, err := storage.OpenReadOnly(dbPath)
	if !errors.Is(err, storage.ErrNoDatabase) {
		t.Fatalf("expected no database, got %v", err)
	}
	_ = store
	s := New(standardTree(), nil, Options{DryRun: true, PublicPrefix: "https://openlist.example.invalid"})
	report, err := s.Run(context.Background(), []Source{sourceFor(root)})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if report.Sources[0].Created != 4 {
		t.Fatalf("dry-run should plan 4 creations: %+v", report.Sources[0])
	}
	if files := collectFiles(t, root); len(files) != 0 {
		t.Fatalf("dry-run created files: %v", files)
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Fatal("dry-run created a database")
	}
}

func TestAPIFailureDoesNotCommitRound(t *testing.T) {
	root := t.TempDir()
	store := newStore(t)
	lister := standardTree()
	lister.listErr["/Movies/科幻"] = errors.New("mid-pagination failure")
	s := New(lister, store, Options{PublicPrefix: "https://openlist.example.invalid"})
	report, err := s.Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("traversal failure must error")
	}
	if !report.Sources[0].Partial {
		t.Fatalf("expected partial: %+v", report.Sources[0])
	}
	if epoch, _ := store.NextEpoch("/Movies"); epoch != 1 {
		t.Fatalf("round must not be committed: epoch=%d", epoch)
	}
}

func TestRootFailureIsNotEmptyDirectory(t *testing.T) {
	root := t.TempDir()
	store := newStore(t)
	lister := standardTree()
	lister.getErr["/Movies"] = errors.New("offline")
	s := New(lister, store, Options{PublicPrefix: "https://openlist.example.invalid"})
	report, err := s.Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("root failure must error")
	}
	if report.Sources[0].Filtered != 0 {
		t.Fatalf("failure must not look like an empty directory: %+v", report.Sources[0])
	}
	if report.Sources[0].Errors == 0 || !report.Sources[0].Partial {
		t.Fatalf("root failure must be reported as an error/partial result: %+v", report.Sources[0])
	}
}

func TestSourceRootMustBeDirectory(t *testing.T) {
	root := t.TempDir()
	store := newStore(t)
	lister := standardTree()
	lister.rootFile = true
	report, err := New(lister, store, Options{PublicPrefix: "https://openlist.example.invalid"}).Run(
		context.Background(), []Source{sourceFor(root)},
	)
	if err == nil || report.Sources[0].Errors == 0 || !report.Sources[0].Partial {
		t.Fatalf("file-shaped source root must fail: report=%+v err=%v", report, err)
	}
	if epoch, err := store.NextEpoch("/Movies"); err != nil || epoch != 1 {
		t.Fatalf("invalid source root committed a round: epoch=%d err=%v", epoch, err)
	}
}

type rootSwapLister struct {
	*fakeLister
	onList func()
}

func (l *rootSwapLister) List(ctx context.Context, path string) ([]openlist.Entry, error) {
	if l.onList != nil {
		l.onList()
		l.onList = nil
	}
	return l.fakeLister.List(ctx, path)
}

func TestOutputRootReplacementDuringHTTPDoesNotEscapeOrCommit(t *testing.T) {
	for _, initiallyExists := range []bool{false, true} {
		name := "missing-root"
		if initiallyExists {
			name = "existing-root"
		}
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "out")
			outside := filepath.Join(base, "outside")
			if err := os.Mkdir(outside, 0o755); err != nil {
				t.Fatal(err)
			}
			if initiallyExists {
				if err := os.Mkdir(root, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			lister := &rootSwapLister{
				fakeLister: &fakeLister{
					dirs:    map[string][]openlist.Entry{"/Movies": {media("A.mkv", 1, "")}},
					getErr:  map[string]error{},
					listErr: map[string]error{},
				},
			}
			lister.onList = func() {
				if initiallyExists {
					moved := filepath.Join(base, "moved-out")
					if err := os.Rename(root, moved); err != nil {
						t.Errorf("move output root: %v", err)
						return
					}
				}
				if err := os.Symlink(outside, root); err != nil {
					t.Errorf("swap output root: %v", err)
				}
			}
			store := newStore(t)
			report, err := New(lister, store, Options{PublicPrefix: "https://openlist.example.invalid"}).Run(
				context.Background(), []Source{sourceFor(root)},
			)
			if err == nil || report.Sources[0].Errors == 0 || !report.Sources[0].Partial {
				t.Fatalf("root replacement must fail partially: report=%+v err=%v", report.Sources[0], err)
			}
			if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
				t.Fatalf("replacement target was modified: entries=%v err=%v", entries, err)
			}
			if epoch, err := store.NextEpoch("/Movies"); err != nil || epoch != 1 {
				t.Fatalf("replacement advanced the round: epoch=%d err=%v", epoch, err)
			}
			if _, ok, err := store.Lookup("/Movies", "A.strm"); err != nil || ok {
				t.Fatalf("replacement fabricated ownership: ok=%t err=%v", ok, err)
			}
		})
	}
}

func TestDiagnosticsAreSampledAndCountOmissions(t *testing.T) {
	var result SourceResult
	for i := 0; i < maxDiagnosticSamples+17; i++ {
		result.addDiagnostic(fmt.Sprintf("diagnostic %d", i))
	}
	if len(result.Diagnostics) != maxDiagnosticSamples || result.DiagnosticsOmitted != 17 {
		t.Fatalf("diagnostics were not bounded: samples=%d omitted=%d", len(result.Diagnostics), result.DiagnosticsOmitted)
	}
}

type fifoSwapFS struct {
	LocalFS
	swapped bool
}

func (f *fifoSwapFS) ReadFile(root, rel string) ([]byte, error) {
	if !f.swapped {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.Remove(path); err != nil {
			return nil, err
		}
		if err := unix.Mkfifo(path, 0o600); err != nil {
			return nil, err
		}
		f.swapped = true
	}
	return f.LocalFS.ReadFile(root, rel)
}

func TestReadFileRaceToFIFOIsBoundedAndDoesNotCommit(t *testing.T) {
	root := t.TempDir()
	store := newStore(t)
	lister := &fakeLister{
		dirs:    map[string][]openlist.Entry{"/Movies": {media("A.mkv", 1, "")}},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
	options := Options{PublicPrefix: "https://openlist.example.invalid"}
	if _, err := New(lister, store, options).Run(context.Background(), []Source{sourceFor(root)}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	options.FS = &fifoSwapFS{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	report, err := New(lister, store, options).Run(ctx, []Source{sourceFor(root)})
	if err == nil || ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("FIFO swap must fail promptly without completing a round: err=%v ctx=%v", err, ctx.Err())
	}
	if report.Sources[0].Conflicts == 0 || !report.Sources[0].Partial {
		t.Fatalf("FIFO swap was not reported as a conflict: %+v", report.Sources[0])
	}
	if epoch, err := store.NextEpoch("/Movies"); err != nil || epoch != 2 {
		t.Fatalf("failed FIFO read advanced round: epoch=%d err=%v", epoch, err)
	}
}

func TestPublishFailureIsReportedAndPartial(t *testing.T) {
	root := t.TempDir()
	store := newStore(t)
	lister := &fakeLister{
		dirs: map[string][]openlist.Entry{
			"/Movies": {media("A.mkv", 1, shaA), media("B.mkv", 2, shaB)},
		},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
	s := New(lister, store, Options{PublicPrefix: "https://openlist.example.invalid", FS: &failingFS{failOn: "B.strm"}})
	report, err := s.Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("publish failure must error")
	}
	if report.Sources[0].Errors != 1 {
		t.Fatalf("expected one error: %+v", report.Sources[0])
	}
	// A already published and owned; B absent and unowned.
	if _, ok, _ := store.Lookup("/Movies", "A.strm"); !ok {
		t.Fatal("successfully published file should remain owned")
	}
	if _, ok, _ := store.Lookup("/Movies", "B.strm"); ok {
		t.Fatal("failed file must not be owned")
	}
	if _, err := os.Stat(filepath.Join(root, "B.strm")); !os.IsNotExist(err) {
		t.Fatal("failed target must not exist")
	}
}

func TestConcurrentTargetAppearanceIsNotOverwritten(t *testing.T) {
	root := t.TempDir()
	store := newStore(t)
	lister := &fakeLister{
		dirs:    map[string][]openlist.Entry{"/Movies": {media("A.mkv", 1, shaA)}},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
	// The injected FS materialises an external file exactly when publish runs,
	// simulating a target that appears after the stat but before the link.
	s := New(lister, store, Options{PublicPrefix: "https://openlist.example.invalid", FS: &racingFS{}})
	report, err := s.Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("concurrent appearance must not be silently overwritten")
	}
	if report.Sources[0].Errors != 1 {
		t.Fatalf("expected one error: %+v", report.Sources[0])
	}
	raw, _ := os.ReadFile(filepath.Join(root, "A.strm"))
	if string(raw) != "external" {
		t.Fatalf("external file was overwritten: %q", raw)
	}
	if _, ok, _ := store.Lookup("/Movies", "A.strm"); ok {
		t.Fatal("racing target must not be owned")
	}
}

// racingFS creates the target just before delegating to the real publish.
type racingFS struct{ LocalFS }

func (f *racingFS) Publish(root, rel string, data []byte) error {
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte("external"), 0o644); err != nil {
		return err
	}
	return f.LocalFS.Publish(root, rel, data)
}

func TestWideDirectoryCompletes(t *testing.T) {
	const width = 500
	var entries []openlist.Entry
	for i := 0; i < width; i++ {
		entries = append(entries, media(fmt.Sprintf("F%04d.mkv", i), int64(i), ""))
	}
	lister := &fakeLister{dirs: map[string][]openlist.Entry{"/Movies": entries}, getErr: map[string]error{}, listErr: map[string]error{}}
	root := t.TempDir()
	store := newStore(t)
	s := New(lister, store, Options{PublicPrefix: "https://openlist.example.invalid", Workers: 4})
	report, err := s.Run(context.Background(), []Source{sourceFor(root)})
	if err != nil {
		t.Fatalf("wide run: %v", err)
	}
	if report.Sources[0].Created != width {
		t.Fatalf("created %d want %d", report.Sources[0].Created, width)
	}
	if files := collectFiles(t, root); len(files) != width {
		t.Fatalf("expected %d files, got %d", width, len(files))
	}
}

func TestDeepDirectoryCompletes(t *testing.T) {
	lister := &fakeLister{dirs: map[string][]openlist.Entry{}, getErr: map[string]error{}, listErr: map[string]error{}}
	remote, rel := "/Movies", ""
	for i := 0; i < 40; i++ {
		child := fmt.Sprintf("d%02d", i)
		lister.dirs[remote] = []openlist.Entry{dir(child), media("F.mkv", int64(i), "")}
		remote = remote + "/" + child
		rel = rel + "/" + child
	}
	lister.dirs[remote] = []openlist.Entry{media("Leaf.mkv", 1, "")}
	root := t.TempDir()
	store := newStore(t)
	s := New(lister, store, Options{PublicPrefix: "https://openlist.example.invalid", Workers: 3})
	report, err := s.Run(context.Background(), []Source{sourceFor(root)})
	if err != nil {
		t.Fatalf("deep run: %v", err)
	}
	if report.Sources[0].Created != 41 {
		t.Fatalf("created %d want 41", report.Sources[0].Created)
	}
}

func TestPartialPublishKeepsEarlierFiles(t *testing.T) {
	root := t.TempDir()
	store := newStore(t)
	lister := &fakeLister{
		dirs:    map[string][]openlist.Entry{"/Movies": {media("A.mkv", 1, shaA)}},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
	s := New(lister, &failingOwnershipStore{Store: store}, Options{PublicPrefix: "https://openlist.example.invalid"})
	report, err := s.Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("ownership failure must error")
	}
	if report.Sources[0].Errors == 0 {
		t.Fatalf("expected error: %+v", report.Sources[0])
	}
	if _, statErr := os.Stat(filepath.Join(root, "A.strm")); statErr != nil {
		t.Fatal("published file must remain on disk")
	}
	if _, ok, _ := store.Lookup("/Movies", "A.strm"); ok {
		t.Fatal("unrecorded file must not be owned")
	}
}

func TestRoundCommitFailureIsCountedAndDoesNotAdvanceRound(t *testing.T) {
	root := t.TempDir()
	store := newStore(t)
	lister := &fakeLister{
		dirs:    map[string][]openlist.Entry{"/Movies": {}},
		getErr:  map[string]error{},
		listErr: map[string]error{},
	}
	s := New(lister, &failingRoundCommitStore{Store: store},
		Options{PublicPrefix: "https://openlist.example.invalid"})
	report, err := s.Run(context.Background(), []Source{sourceFor(root)})
	if err == nil {
		t.Fatal("round commit failure must fail the source")
	}
	result := report.Sources[0]
	if result.Errors != 1 || !result.Partial {
		t.Fatalf("round commit failure must be counted and partial: %+v", result)
	}
	if epoch, err := store.NextEpoch("/Movies"); err != nil || epoch != 1 {
		t.Fatalf("failed commit advanced round: epoch=%d err=%v", epoch, err)
	}
}

// failingFS injects a publish failure for one relative path.
type failingFS struct {
	LocalFS
	failOn string
}

func (f *failingFS) Publish(root, rel string, data []byte) error {
	if strings.HasSuffix(rel, f.failOn) {
		return errors.New("injected publish failure")
	}
	return f.LocalFS.Publish(root, rel, data)
}

// failingOwnershipStore injects an ownership-recording failure.
type failingOwnershipStore struct{ *storage.Store }

func (f *failingOwnershipStore) RecordOwnership(context.Context, storage.Ownership, int64) error {
	return errors.New("injected ownership failure")
}

type failingRoundCommitStore struct{ *storage.Store }

func (f *failingRoundCommitStore) CommitRound(context.Context, string, int64, time.Time) error {
	return errors.New("injected round commit failure")
}

func collectFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(root, path)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

func statTimes(t *testing.T, root string) map[string]time.Time {
	t.Helper()
	out := map[string]time.Time{}
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, _ := d.Info()
		rel, _ := filepath.Rel(root, path)
		out[filepath.ToSlash(rel)] = info.ModTime()
		return nil
	})
	return out
}
