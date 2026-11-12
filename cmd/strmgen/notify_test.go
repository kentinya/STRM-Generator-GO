package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"strm-generator-go/internal/config"
	"strm-generator-go/internal/notify"
	"strm-generator-go/internal/openlist"
	"strm-generator-go/internal/reconciler"
	"strm-generator-go/internal/scheduler"
	"strm-generator-go/internal/storage"
)

// notifyTempDirs lists the notification temporary directories of this user.
func notifyTempDirs(t *testing.T) map[string]bool {
	t.Helper()
	base := os.TempDir()
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "strmgen-notify-") {
			found[entry.Name()] = true
		}
	}
	return found
}

func testNotifier(t *testing.T) *notifier {
	t.Helper()
	t.Setenv("TELEGRAM_BOT_TOKEN", "unit-token")
	t.Setenv("TELEGRAM_CHAT_ID", "42")
	t.Setenv("OPENLIST_TOKEN", "x")
	resolved, err := config.Load(writeNotificationConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	n := setupNotifications(resolved, false, io.Discard)
	if !n.enabled {
		t.Fatal("notifier must be enabled")
	}
	return n
}

func writeNotificationConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "openlist:\n" +
		"  api_url: \"http://127.0.0.1:1\"\n" +
		"  public_url: \"https://openlist.example.invalid\"\n" +
		"  token_env: \"OPENLIST_TOKEN\"\n" +
		"sources:\n  - remote: \"/Movies\"\n    output: \"" + dir + "/out\"\n" +
		"database:\n  path: \"" + dir + "/db.sqlite\"\n" +
		"notifications:\n  telegram:\n    enabled: true\n    policy: \"all\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestSendRoundCleansTemporaryArtifacts proves that a busy-skip round and a
// cancelled round never leave a notification temporary directory behind.
func TestSendRoundCleansTemporaryArtifacts(t *testing.T) {
	n := testNotifier(t)

	before := notifyTempDirs(t)
	collector := n.newCollector("sync")
	if collector == nil {
		t.Fatal("collector missing")
	}
	n.sendRound(context.Background(), "sync", time.Now(), time.Second, nil, scheduler.Result{Busy: true}, collector)
	after := notifyTempDirs(t)
	for name := range after {
		if !before[name] {
			t.Fatalf("a busy-skip round left %s behind", name)
		}
	}

	collector = n.newCollector("sync")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n.sendRound(ctx, "sync", time.Now(), time.Second, nil, scheduler.Result{}, collector)
	after = notifyTempDirs(t)
	for name := range after {
		if !before[name] {
			t.Fatalf("a cancelled round left %s behind", name)
		}
	}
}

// failingReceiver is a local transport that always rejects the summary, so
// delivery-failure paths never touch the real network.
func failingReceiver(t *testing.T) *notify.Sender {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	return notify.NewSenderWithBase("unit-token", "42", server.URL)
}

// TestSendRoundCleansUpAfterDeliveryFailure proves a failed transport still
// cleans the listing and keeps the process side-effect free.
func TestSendRoundCleansUpAfterDeliveryFailure(t *testing.T) {
	n := testNotifier(t)
	n.sender = failingReceiver(t)
	before := notifyTempDirs(t)
	collector := n.newCollector("sync")
	if collector == nil {
		t.Fatal("collector missing")
	}
	collector.Record(notify.Event{SourceIndex: 0, Kind: notify.KindCreated, RelativePath: "A.strm"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	n.sendRound(ctx, "sync", time.Now(), time.Second,
		&reconciler.Report{Sources: []reconciler.SourceResult{{SourceIndex: 0, Created: 1}}}, scheduler.Result{}, collector)
	after := notifyTempDirs(t)
	for name := range after {
		if !before[name] {
			t.Fatalf("a failed delivery left %s behind", name)
		}
	}
}

// TestSendRoundWithoutCollectorStillSendsDegradedSummary proves that a detail
// collection failure only degrades the notification: the summary is still
// sent under the policy with an explicit unknown-progress note.
func TestSendRoundWithoutCollectorStillSendsDegradedSummary(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)
	n := testNotifier(t)
	n.sender = notify.NewSenderWithBase("unit-token", "42", server.URL)
	n.sendRound(context.Background(), "sync", time.Now(), time.Second,
		&reconciler.Report{Sources: []reconciler.SourceResult{{SourceIndex: 0, Skipped: 1}}}, scheduler.Result{}, nil)
	if requests != 1 {
		t.Fatalf("requests = %d, want exactly one degraded summary", requests)
	}

	// A change_or_error policy with no collector and no report change must
	// stay silent; a failing round still sends.
	n.policy = "change_or_error"
	n.sendRound(context.Background(), "sync", time.Now(), time.Second,
		&reconciler.Report{Sources: []reconciler.SourceResult{{SourceIndex: 0, Skipped: 1}}}, scheduler.Result{}, nil)
	if requests != 1 {
		t.Fatalf("requests = %d, an unchanged round without detail must stay silent", requests)
	}
	n.sendRound(context.Background(), "sync", time.Now(), time.Second,
		&reconciler.Report{Sources: []reconciler.SourceResult{{SourceIndex: 0, Errors: 1, Partial: true}}}, scheduler.Result{}, nil)
	if requests != 2 {
		t.Fatalf("requests = %d, a failing round must send without detail", requests)
	}
}

// n_newCollector creates a real round collector for the production-chain
// tests.
func n_newCollector(t *testing.T) *notify.Collector {
	t.Helper()
	c, err := notify.NewCollector("sync "+t.Name(), 3)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// notifyLister is a minimal local OpenList stand-in for the production-chain
// tests below.
type notifyLister struct {
	entries []openlist.Entry
	getErr  map[string]error
}

func (f *notifyLister) Get(_ context.Context, path string) (*openlist.Entry, error) {
	if err, ok := f.getErr[path]; ok {
		return nil, err
	}
	return &openlist.Entry{Name: filepath.Base(path), IsDir: true}, nil
}

func (f *notifyLister) List(_ context.Context, _ string) ([]openlist.Entry, error) {
	return f.entries, nil
}

func notifyMedia(name string, sha string) openlist.Entry {
	size := int64(10)
	e := openlist.Entry{Name: name, Size: &size}
	if sha != "" {
		e.SHA1 = sha
		e.HasSHA = true
	}
	return e
}

// commitFailStore wraps the real store so one commit fails inside the real
// transaction after the first advance has been staged: the durable UPDATE
// never applies and the sink discard path runs with real staged records.
type commitFailStore struct {
	*storage.Store
	failNext bool
}

func (w *commitFailStore) CommitRound(ctx context.Context, key string, epoch int64, at time.Time, sink storage.AdvanceSink) (storage.CommitResult, error) {
	if !w.failNext {
		return w.Store.CommitRound(ctx, key, epoch, at, sink)
	}
	w.failNext = false
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	return w.Store.CommitRound(ctx, key, epoch, at, &cancelAfterFirstStage{AdvanceSink: sink, cancel: cancel})
}

type cancelAfterFirstStage struct {
	storage.AdvanceSink
	cancel context.CancelFunc
}

func (c *cancelAfterFirstStage) StageAdvance(a storage.MissingAdvance) {
	c.AdvanceSink.StageAdvance(a)
	c.cancel()
}

// TestMissingAdvancesFollowRealCommitFacts runs the production chain — real
// store, real syncer, production fact adapter, real collector — over the
// actual commit windows. A committed advance is published with its durable
// count; a failed transaction must leave neither the database nor the
// notification with the uncommitted candidate (B re-review B-R1).
func TestMissingAdvancesFollowRealCommitFacts(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(dir, "data", "strmgen.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	outRoot := filepath.Join(dir, "out")
	if err := os.MkdirAll(outRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	lister := &notifyLister{
		entries: []openlist.Entry{notifyMedia("A.mkv", strings.Repeat("a", 40))},
		getErr:  map[string]error{},
	}
	opts := reconciler.Options{
		PublicPrefix:         "https://openlist.example.invalid",
		CleanupEnabled:       true,
		MissingConfirmations: 3,
	}
	source := reconciler.Source{Index: 0, Key: "/Movies", Remote: "/Movies", Output: outRoot}
	report, err := reconciler.New(lister, store, opts).Run(context.Background(), []reconciler.Source{source})
	if err != nil || report.Sources[0].Created != 1 {
		t.Fatalf("seed round: %+v err=%v", report, err)
	}

	// The remote target becomes unreadable: the next round commits 0→1.
	lister.entries = nil
	lister.getErr["/Movies/A.mkv"] = errors.New("target not read")
	collector := n_newCollector(t)
	defer collector.Close()
	adapter := factSinkAdapter{collector: collector}
	opts.Facts = adapter
	wrapped := &commitFailStore{Store: store}
	report, err = reconciler.New(lister, wrapped, opts).Run(context.Background(), []reconciler.Source{source})
	if err != nil || report.Sources[0].Partial {
		t.Fatalf("first missing round: %+v err=%v", report, err)
	}
	o, ok, err := store.Lookup("/Movies", "A.strm")
	if err != nil || !ok || o.MissingCount != 1 {
		t.Fatalf("durable count after committed round: %+v ok=%v err=%v", o, ok, err)
	}
	if collector.TotalEvents() != 1 || !collector.HasChange() {
		t.Fatalf("committed advance must be visible: total=%d change=%v", collector.TotalEvents(), collector.HasChange())
	}
	advances := collector.Preview()
	if len(advances) != 1 || advances[0].Kind != notify.KindMissingAdvanced ||
		advances[0].PriorCount != 0 || advances[0].NewCount != 1 || advances[0].Reason != notify.ReasonSourceTarget {
		t.Fatalf("advance event: %+v", advances)
	}

	// The second missing round stages 1→2 but its real transaction fails;
	// neither the database nor the collector may report the candidate.
	wrapped.failNext = true
	failed := n_newCollector(t)
	defer failed.Close()
	opts.Facts = factSinkAdapter{failed}
	report, err = reconciler.New(lister, wrapped, opts).Run(context.Background(), []reconciler.Source{source})
	if err == nil || !report.Sources[0].Partial {
		t.Fatalf("failed commit must fail the round: %+v err=%v", report, err)
	}
	o, _, err = store.Lookup("/Movies", "A.strm")
	if err != nil || o.MissingCount != 1 {
		t.Fatalf("a failed commit must not change the durable count: %+v err=%v", o, err)
	}
	if failed.TotalEvents() != 0 || failed.HasChange() || failed.Degraded() {
		t.Fatalf("uncommitted candidate must be fully withdrawn: total=%d change=%v degraded=%v",
			failed.TotalEvents(), failed.HasChange(), failed.Degraded())
	}

	// The committed-round summary reports the real progress; the failed round
	// never mentions the uncommitted 1→2 candidate.
	info := buildRunInfo(testNotifier(t), "sync", time.Now(), time.Second, report)
	info.DetailUnavailable = false
	text, _ := notify.BuildMessage(info, failed)
	if strings.Contains(text, "1→2") {
		t.Fatalf("the failed round must not report the uncommitted advance:\n%s", text)
	}
	committedInfo := notify.RunInfo{
		InstanceName: "S", Mode: "sync", Outcome: notify.OutcomeSourceFailure,
		SourceCount: 2, IncompleteSources: 1, StatsAvailable: true, Threshold: 3,
	}
	// One overflowing source forces the single attachment; the committed
	// advance must appear in the body preview and in the listing.
	longNote := strings.Repeat("原因说明", 1100)
	committedInfo.Sources = []notify.SourceStatus{{Index: 1, Notes: []string{longNote}, DetailNotes: []string{longNote}}}
	committedText, attachment := notify.BuildMessage(committedInfo, collector)
	if attachment == "" {
		t.Fatal("the source overflow must produce the single attachment")
	}
	if !strings.Contains(committedText, "0→1/阈值 3") {
		t.Fatalf("committed progress missing from the body:\n%s", committedText)
	}
	data, err := os.ReadFile(attachment)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "缺失计数 A.strm 计数 0→1/阈值 3") {
		t.Fatalf("the committed advance must reach the listing:\n%s", data)
	}
}
