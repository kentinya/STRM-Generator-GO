//go:build strmgen_testhooks

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// telegramReceiver is a minimal local stand-in for the Bot API used only by
// test builds. It records the raw requests so assertions can check the exact
// endpoint, the chat id and the message bodies without ever contacting the
// real Telegram network.
type telegramReceiver struct {
	mu       sync.Mutex
	server   *httptest.Server
	requests []capturedRequest
}

type capturedRequest struct {
	path      string
	content   string
	multipart bool
	chatID    string
}

func newTelegramReceiver(t *testing.T) *telegramReceiver {
	t.Helper()
	r := &telegramReceiver{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		captured := capturedRequest{path: req.URL.Path, content: string(body)}
		ct := req.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "multipart/") {
			captured.multipart = true
			if _, params, err := mime.ParseMediaType(ct); err == nil {
				form, err := multipart.NewReader(strings.NewReader(captured.content), params["boundary"]).ReadForm(1 << 22)
				if err == nil {
					if v := form.Value["chat_id"]; len(v) > 0 {
						captured.chatID = v[0]
					}
					if f := form.File["document"]; len(f) > 0 {
						fh, err := f[0].Open()
						if err == nil {
							data, _ := io.ReadAll(fh)
							fh.Close()
							captured.content = string(data)
						}
					}
				}
			}
		} else {
			values, err := url.ParseQuery(captured.content)
			if err != nil {
				t.Errorf("parse form body: %v", err)
			}
			if v := values["chat_id"]; len(v) > 0 {
				captured.chatID = v[0]
			}
			if v := values["text"]; len(v) > 0 {
				captured.content = v[0]
			}
		}
		r.mu.Lock()
		r.requests = append(r.requests, captured)
		r.mu.Unlock()
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	})
	r.server = httptest.NewServer(mux)
	t.Cleanup(r.server.Close)
	return r
}

func (r *telegramReceiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

func (r *telegramReceiver) all() []capturedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]capturedRequest(nil), r.requests...)
}

func (r *telegramReceiver) sentDocuments() []capturedRequest {
	var out []capturedRequest
	for _, req := range r.all() {
		if req.multipart {
			out = append(out, req)
		}
	}
	return out
}

func (r *telegramReceiver) sentMessages() []capturedRequest {
	var out []capturedRequest
	for _, req := range r.all() {
		if !req.multipart {
			out = append(out, req)
		}
	}
	return out
}

// startMultiFileAPI serves one directory with count media files.
func startMultiFileAPI(t *testing.T, count int) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/fs/get", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Path string }
		json.NewDecoder(r.Body).Decode(&req)
		writeJSON(w, map[string]any{"name": "Movies", "is_dir": true, "size": 0, "hash_info": nil}, 200)
	})
	mux.HandleFunc("/api/fs/list", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Path string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.Path != "/Movies" {
			writeJSON(w, map[string]any{"content": []any{}, "total": 0}, 200)
			return
		}
		entries := make([]map[string]any, 0, count)
		for i := 0; i < count; i++ {
			entries = append(entries, map[string]any{
				"name": fmt.Sprintf("F%02d.mkv", i), "is_dir": false, "size": 10,
				"hash_info": map[string]any{"sha1": strings.Repeat("a", 40)},
			})
		}
		writeJSON(w, map[string]any{"content": entries, "total": count}, 200)
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func writeJSON(w http.ResponseWriter, data any, code int) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"code": code, "message": "m", "data": data})
}

func writeNotifyConfig(t *testing.T, apiURL, root, db, policy string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "openlist:\n" +
		"  api_url: \"" + apiURL + "\"\n" +
		"  public_url: \"https://openlist.example.invalid\"\n" +
		"  token_env: \"OPENLIST_TOKEN\"\n" +
		"sources:\n  - remote: \"/Movies\"\n    output: \"" + root + "\"\n" +
		"cleanup:\n  enabled: false\n" +
		"database:\n  path: \"" + db + "\"\n" +
		"notifications:\n  telegram:\n    enabled: true\n" +
		"    instance_name: \"家庭媒体\"\n" +
		"    policy: \"" + policy + "\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func setTelegramEnv(t *testing.T, token, chat string) {
	t.Helper()
	t.Setenv("TELEGRAM_BOT_TOKEN", token)
	t.Setenv("TELEGRAM_CHAT_ID", chat)
	t.Setenv("OPENLIST_TOKEN", "fixture-token")
}

// runSyncForTest invokes the real sync entry with the test API base override.
func runSyncForTest(t *testing.T, configPath string, receiver *telegramReceiver) int {
	t.Helper()
	t.Setenv("STRMGEN_TEST_TELEGRAM_API_BASE", receiver.server.URL)
	return run([]string{"sync", "--config", configPath}, io.Discard, os.Stderr)
}

func TestSyncNotificationSendsOneSummaryPerRun(t *testing.T) {
	receiver := newTelegramReceiver(t)
	api := startMultiFileAPI(t, 3)
	base := t.TempDir()
	root := filepath.Join(base, "out")
	db := filepath.Join(base, "data", "strmgen.db")
	setTelegramEnv(t, "test-token", "-10042")
	configPath := writeNotifyConfig(t, api.URL, root, db, "all")

	if code := runSyncForTest(t, configPath, receiver); code != 0 {
		t.Fatalf("sync exit = %d", code)
	}
	if code := runSyncForTest(t, configPath, receiver); code != 0 {
		t.Fatalf("second sync exit = %d", code)
	}
	messages := receiver.sentMessages()
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want exactly one per sync run", len(messages))
	}
	first := messages[0]
	if !strings.Contains(first.path, "/bottest-token/sendMessage") {
		t.Fatalf("endpoint = %s", first.path)
	}
	if first.chatID != "-10042" {
		t.Fatalf("chat = %q", first.chatID)
	}
	if !strings.Contains(first.content, "家庭媒体 STRM 同步报告") {
		t.Fatalf("body header missing:\n%s", first.content)
	}
	if !strings.Contains(first.content, "新增 3") {
		t.Fatalf("first run must report the created files:\n%s", first.content)
	}
	if !strings.Contains(first.content, "结果：成功") {
		t.Fatalf("first run result:\n%s", first.content)
	}
	second := messages[1]
	if !strings.Contains(second.content, "本轮无新增、更新、删除或归属回收事件") {
		t.Fatalf("second run body:\n%s", second.content)
	}
	if receiver.sentDocuments() != nil {
		t.Fatal("a fitting round must not upload an attachment")
	}
	// The summary must never carry the token, the chat id or the OpenList URL.
	for _, m := range messages {
		if strings.Contains(m.content, "test-token") || strings.Contains(m.content, "-10042") ||
			strings.Contains(m.content, api.URL) || strings.Contains(m.content, "fixture-token") {
			t.Fatalf("summary leaks a secret or private endpoint:\n%s", m.content)
		}
	}
}

func TestSyncNotificationAttachmentBeyondPreviewLimit(t *testing.T) {
	receiver := newTelegramReceiver(t)
	api := startMultiFileAPI(t, 30)
	base := t.TempDir()
	root := filepath.Join(base, "out")
	db := filepath.Join(base, "data", "strmgen.db")
	setTelegramEnv(t, "test-token", "-10042")
	configPath := writeNotifyConfig(t, api.URL, root, db, "all")

	if code := runSyncForTest(t, configPath, receiver); code != 0 {
		t.Fatalf("sync exit = %d", code)
	}
	if receiver.count() != 2 {
		t.Fatalf("requests = %d, want one summary plus one attachment", receiver.count())
	}
	docs := receiver.sentDocuments()
	if len(docs) != 1 {
		t.Fatalf("documents = %d", len(docs))
	}
	content := docs[0].content
	if !strings.Contains(content, "事件总数: 30") {
		t.Fatalf("attachment trailer missing:\n%s", content)
	}
	if !strings.Contains(docs[0].chatID, "-10042") {
		t.Fatalf("attachment chat = %q", docs[0].chatID)
	}
	for i := 0; i < 30; i++ {
		if !strings.Contains(content, fmt.Sprintf("来源 0 新增 F%02d.strm", i)) {
			t.Fatalf("attachment misses F%02d.strm", i)
		}
	}
}

func TestSyncNotificationMissingCredentialsWarnsAndSyncs(t *testing.T) {
	receiver := newTelegramReceiver(t)
	api := startMultiFileAPI(t, 2)
	base := t.TempDir()
	root := filepath.Join(base, "out")
	db := filepath.Join(base, "data", "strmgen.db")
	t.Setenv("OPENLIST_TOKEN", "fixture-token")
	t.Setenv("STRMGEN_TEST_TELEGRAM_API_BASE", receiver.server.URL)
	configPath := writeNotifyConfig(t, api.URL, root, db, "all")

	stderr := &strings.Builder{}
	code := run([]string{"sync", "--config", configPath}, io.Discard, stderr)
	if code != 0 {
		t.Fatalf("sync exit = %d", code)
	}
	if !strings.Contains(stderr.String(), "notifications are disabled for this process") {
		t.Fatalf("warning missing: %s", stderr.String())
	}
	if strings.Contains(stderr.String(), "test-token") {
		t.Fatalf("warning echoes a credential: %s", stderr.String())
	}
	if receiver.count() != 0 {
		t.Fatalf("requests = %d, want none", receiver.count())
	}
}

func TestSyncDryRunSendsNothingAndSkipsCredentialCheck(t *testing.T) {
	receiver := newTelegramReceiver(t)
	api := startMultiFileAPI(t, 2)
	base := t.TempDir()
	root := filepath.Join(base, "out")
	db := filepath.Join(base, "data", "strmgen.db")
	t.Setenv("OPENLIST_TOKEN", "fixture-token")
	// Deliberately no TELEGRAM_* variables: dry-run must not require them.
	t.Setenv("STRMGEN_TEST_TELEGRAM_API_BASE", receiver.server.URL)
	configPath := writeNotifyConfig(t, api.URL, root, db, "all")

	code := run([]string{"sync", "--config", configPath, "--dry-run"}, io.Discard, io.Discard)
	if code != 0 {
		t.Fatalf("dry-run exit = %d", code)
	}
	if receiver.count() != 0 {
		t.Fatalf("dry-run sent %d requests", receiver.count())
	}
}

func TestSyncNotificationPolicyErrorOnlySendsFailures(t *testing.T) {
	receiver := newTelegramReceiver(t)
	api := startMultiFileAPI(t, 2)
	base := t.TempDir()
	root := filepath.Join(base, "out")
	db := filepath.Join(base, "data", "strmgen.db")
	setTelegramEnv(t, "test-token", "-10042")
	configPath := writeNotifyConfig(t, api.URL, root, db, "error")

	if code := runSyncForTest(t, configPath, receiver); code != 0 {
		t.Fatalf("success exit = %d", code)
	}
	if receiver.count() != 0 {
		t.Fatalf("a successful round must not send under the error policy (requests=%d)", receiver.count())
	}

	// Break the OpenList API: the next round fails and must trigger a send.
	api.Close()
	if code := runSyncForTest(t, configPath, receiver); code != 1 {
		t.Fatalf("failure exit = %d", code)
	}
	messages := receiver.sentMessages()
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want exactly the failure summary", len(messages))
	}
	if !strings.Contains(messages[0].content, "未完成") {
		t.Fatalf("failure body:\n%s", messages[0].content)
	}
	if strings.Contains(messages[0].content, "统计：不可用") {
		t.Fatalf("a source failure has a report; counts must not be unavailable:\n%s", messages[0].content)
	}
}

func TestSyncNotificationPolicyChangeOrErrorSkipsUnchangedRounds(t *testing.T) {
	receiver := newTelegramReceiver(t)
	api := startMultiFileAPI(t, 2)
	base := t.TempDir()
	root := filepath.Join(base, "out")
	db := filepath.Join(base, "data", "strmgen.db")
	setTelegramEnv(t, "test-token", "-10042")
	configPath := writeNotifyConfig(t, api.URL, root, db, "change_or_error")

	if code := runSyncForTest(t, configPath, receiver); code != 0 {
		t.Fatalf("first exit = %d", code)
	}
	if receiver.count() != 1 {
		t.Fatalf("the change round must send (requests=%d)", receiver.count())
	}
	if code := runSyncForTest(t, configPath, receiver); code != 0 {
		t.Fatalf("second exit = %d", code)
	}
	if receiver.count() != 1 {
		t.Fatalf("an unchanged round must not send under change_or_error (requests=%d)", receiver.count())
	}
}

// startCleanupAPI serves one file that the test can make disappear.
func startCleanupAPI(t *testing.T) (*httptest.Server, *bool) {
	t.Helper()
	mux := http.NewServeMux()
	var present = true
	mux.HandleFunc("/api/fs/get", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Path string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.Path == "/Movies/Gone.mkv" && !present {
			writeJSON(w, nil, 404)
			return
		}
		writeJSON(w, map[string]any{"name": "Movies", "is_dir": true, "size": 0, "hash_info": nil}, 200)
	})
	mux.HandleFunc("/api/fs/list", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Path string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.Path != "/Movies" {
			writeJSON(w, map[string]any{"content": []any{}, "total": 0}, 200)
			return
		}
		entries := []any{}
		if present {
			entries = append(entries, map[string]any{
				"name": "Gone.mkv", "is_dir": false, "size": 10,
				"hash_info": map[string]any{"sha1": strings.Repeat("b", 40)},
			})
		}
		writeJSON(w, map[string]any{"content": entries, "total": len(entries)}, 200)
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, &present
}

func writeCleanupConfig(t *testing.T, apiURL, root, db string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "openlist:\n" +
		"  api_url: \"" + apiURL + "\"\n" +
		"  public_url: \"https://openlist.example.invalid\"\n" +
		"  token_env: \"OPENLIST_TOKEN\"\n" +
		"sources:\n  - remote: \"/Movies\"\n    output: \"" + root + "\"\n" +
		"sync:\n  mirror: true\n" +
		"cleanup:\n  enabled: true\n  missing_confirmations: 3\n" +
		"database:\n  path: \"" + db + "\"\n" +
		"notifications:\n  telegram:\n    enabled: true\n    instance_name: \"家庭媒体\"\n" +
		"    policy: \"all\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSyncNotificationReportsMissingProgressAndDeletion(t *testing.T) {
	receiver := newTelegramReceiver(t)
	api, present := startCleanupAPI(t)
	base := t.TempDir()
	root := filepath.Join(base, "out")
	db := filepath.Join(base, "data", "strmgen.db")
	setTelegramEnv(t, "test-token", "-10042")
	configPath := writeCleanupConfig(t, api.URL, root, db)

	if code := runSyncForTest(t, configPath, receiver); code != 0 {
		t.Fatalf("seed exit = %d", code)
	}
	*present = false
	// Three full successful rounds advance the committed count to the
	// threshold; the fourth round re-checks and deletes.
	for round := 1; round <= 4; round++ {
		if code := runSyncForTest(t, configPath, receiver); code != 0 {
			t.Fatalf("round %d exit = %d", round, code)
		}
	}
	messages := receiver.sentMessages()
	if len(messages) != 5 {
		t.Fatalf("messages = %d", len(messages))
	}
	if !strings.Contains(messages[1].content, "缺失计数 来源 0 Gone.strm（计数 0→1/阈值 3，原因 源目标未读到，首次进入待删除，文件仍保留）") {
		t.Fatalf("first advance:\n%s", messages[1].content)
	}
	if !strings.Contains(messages[2].content, "计数 1→2/阈值 3，原因 源目标未读到，待删除计数推进，文件仍保留") {
		t.Fatalf("second advance:\n%s", messages[2].content)
	}
	third := messages[3]
	if !strings.Contains(third.content, "计数 2→3/阈值 3") {
		t.Fatalf("third round must reach the threshold:\n%s", third.content)
	}
	if strings.Contains(third.content, "实际删除 1") {
		t.Fatalf("round three must not delete yet:\n%s", third.content)
	}
	last := messages[4]
	if !strings.Contains(last.content, "实际删除 1") {
		t.Fatalf("deletion summary:\n%s", last.content)
	}
	if !strings.Contains(last.content, "实际删除 来源 0 Gone.strm（删除前计数 3/阈值 3，已实际删除，原因 源目标未读到）") {
		t.Fatalf("deleted event missing:\n%s", last.content)
	}
	if _, err := os.Stat(filepath.Join(root, "Gone.strm")); !os.IsNotExist(err) {
		t.Fatal("qualified target was not removed")
	}
}

// TestSyncNotificationDegradedWhenCollectionFails proves that a temporary
// listing creation failure only degrades the notification: the synchronization
// result and exit code stay unchanged and the summary still names the missing
// detail instead of reporting zero events.
func TestSyncNotificationDegradedWhenCollectionFails(t *testing.T) {
	receiver := newTelegramReceiver(t)
	api := startMultiFileAPI(t, 2)
	base := t.TempDir()
	root := filepath.Join(base, "out")
	db := filepath.Join(base, "data", "strmgen.db")
	setTelegramEnv(t, "test-token", "-10042")
	t.Setenv("STRMGEN_TEST_TELEGRAM_API_BASE", receiver.server.URL)
	configPath := writeNotifyConfig(t, api.URL, root, db, "all")

	// The notification collector uses the system temporary directory; a
	// nonexistent TMPDIR makes every creation fail while the synchronization
	// targets stay under the test-controlled roots.
	t.Setenv("TMPDIR", filepath.Join(base, "missing-tmp"))

	stderr := &strings.Builder{}
	code := run([]string{"sync", "--config", configPath}, io.Discard, stderr)
	if code != 0 {
		t.Fatalf("sync exit = %d", code)
	}
	if !strings.Contains(stderr.String(), "detail collection is unavailable") {
		t.Fatalf("degradation warning missing: %s", stderr.String())
	}
	messages := receiver.sentMessages()
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want exactly one degraded summary", len(messages))
	}
	if !strings.Contains(messages[0].content, "明细不完整：本轮明细收集不可用") {
		t.Fatalf("summary must mark the unknown detail:\n%s", messages[0].content)
	}
	if strings.Contains(messages[0].content, "本轮无新增、更新、删除或归属回收事件") {
		t.Fatalf("unknown detail must never read as zero events:\n%s", messages[0].content)
	}
	if !strings.Contains(messages[0].content, "新增 2") {
		t.Fatalf("report counters must still be summarized:\n%s", messages[0].content)
	}
	for _, name := range []string{"F00.strm", "F01.strm"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("%s missing after the degraded notification: %v", name, err)
		}
	}
}
