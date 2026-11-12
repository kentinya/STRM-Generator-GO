package main

import (
	"context"
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
	"strm-generator-go/internal/reconciler"
	"strm-generator-go/internal/scheduler"
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
