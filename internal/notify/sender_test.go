package notify

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// okServer answers every request with the official JSON envelope.
func okServer(t *testing.T, hook func(r *http.Request, body []byte) *string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if reply := hook(r, body); reply != nil {
			_, _ = w.Write([]byte(*reply))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestSendSummaryPostsOfficialEnvelope(t *testing.T) {
	var gotPath string
	var gotChat, gotText string
	ts := okServer(t, func(r *http.Request, body []byte) *string {
		gotPath = r.URL.Path
		values, err := parseValues(r, body)
		if err != nil {
			t.Errorf("parse body: %v", err)
		}
		gotChat = values["chat_id"]
		gotText = values["text"]
		return nil
	})
	s := NewSenderWithBase("123:ABC", "-10042", ts.URL)
	if err := s.Send(context.Background(), "正文", ""); err != nil {
		t.Fatalf("send: %v", err)
	}
	if gotPath != "/bot123:ABC/sendMessage" {
		t.Fatalf("endpoint = %s", gotPath)
	}
	if gotChat != "-10042" || gotText != "正文" {
		t.Fatalf("form = chat %q text %q", gotChat, gotText)
	}
}

func TestSendUploadsSingleMultipartListing(t *testing.T) {
	var sawChat, sawFile, sawContent string
	var uploads int
	ts := okServer(t, func(r *http.Request, body []byte) *string {
		if !strings.HasSuffix(r.URL.Path, "/sendDocument") {
			return nil
		}
		uploads++
		mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
			t.Errorf("content type = %q", r.Header.Get("Content-Type"))
			return nil
		}
		mr := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
		form, err := mr.ReadForm(1 << 20)
		if err != nil {
			t.Errorf("read form: %v", err)
			return nil
		}
		sawChat = form.Value["chat_id"][0]
		file := form.File["document"][0]
		sawFile = file.Filename
		fh, err := file.Open()
		if err != nil {
			t.Errorf("open part: %v", err)
			return nil
		}
		defer fh.Close()
		data, _ := io.ReadAll(fh)
		sawContent = string(data)
		return nil
	})
	dir := t.TempDir()
	listing := filepath.Join(dir, "events.txt")
	if err := os.WriteFile(listing, []byte("STRM 同步事件清单\n运行标识: sync\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewSenderWithBase("123:ABC", "-10042", ts.URL)
	if err := s.Send(context.Background(), "摘要", listing); err != nil {
		t.Fatalf("send: %v", err)
	}
	if uploads != 1 {
		t.Fatalf("uploads = %d", uploads)
	}
	if sawChat != "-10042" || sawFile != "strm-events.txt" {
		t.Fatalf("multipart = chat %q file %q", sawChat, sawFile)
	}
	if !strings.Contains(sawContent, "运行标识: sync") {
		t.Fatalf("listing content = %q", sawContent)
	}
}

func TestSendRejectsApplicationErrorsAndNonSuccessStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		reply  string
	}{
		{"ok false", 200, `{"ok":false,"error_code":400,"description":"secret"}`},
		{"rate limited without retry", 429, `{"ok":false,"error_code":429,"description":"slow"}`},
		{"server error", 500, `internal error`},
		{"invalid json", 200, `not json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			ts := okServer(t, func(*http.Request, []byte) *string {
				requests++
				reply := tc.reply
				return &reply
			})
			// Override the status code with a wrapper.
			srv := ts.Config.Handler
			ts.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				srv.ServeHTTP(w, r)
			})
			s := NewSenderWithBase("t", "c", ts.URL)
			err := s.Send(context.Background(), "text", "")
			if err == nil {
				t.Fatalf("status %d with %q must fail", tc.status, tc.reply)
			}
			if requests != 1 {
				t.Fatalf("requests = %d, want exactly one without retry", requests)
			}
			// The fixed error vocabulary must not echo the response body.
			if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "slow") {
				t.Fatalf("error echoes response detail: %v", err)
			}
		})
	}
}

func TestSendRejectsRedirects(t *testing.T) {
	followed := false
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		followed = true
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer redirectTarget.Close()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL+"/botX/sendMessage", http.StatusFound)
	}))
	defer ts.Close()
	s := NewSenderWithBase("t", "c", ts.URL)
	if err := s.Send(context.Background(), "text", ""); err == nil {
		t.Fatal("a redirect must be rejected")
	}
	if followed {
		t.Fatal("the client followed the redirect")
	}
}

func TestSendCapsOversizedResponses(t *testing.T) {
	// A response larger than the 64 KiB cap is cut off mid-JSON and treated as
	// an invalid result; the reader never grows without bound.
	huge := `{"ok":true,"result":{"padding":"` + strings.Repeat("x", 200*1024) + `"}}`
	requests := 0
	ts := okServer(t, func(*http.Request, []byte) *string {
		requests++
		return &huge
	})
	s := NewSenderWithBase("t", "c", ts.URL)
	if err := s.Send(context.Background(), "text", ""); err == nil {
		t.Fatal("an oversized response must be rejected as invalid")
	}
	if requests != 1 {
		t.Fatalf("requests = %d", requests)
	}
	// A valid response within the cap still succeeds.
	small := `{"ok":true}`
	ts2 := okServer(t, func(*http.Request, []byte) *string { return &small })
	s2 := NewSenderWithBase("t", "c", ts2.URL)
	if err := s2.Send(context.Background(), "text", ""); err != nil {
		t.Fatalf("a valid response within the cap must succeed: %v", err)
	}
}

func TestSendHonoursCancellationAndSharesBudget(t *testing.T) {
	release := make(chan struct{})
	ts := okServer(t, func(*http.Request, []byte) *string {
		<-release
		return nil
	})
	defer func() { close(release); ts.Close() }()
	s := NewSenderWithBase("t", "c", ts.URL)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := s.Send(ctx, "text", "")
	if err == nil {
		t.Fatal("a cancelled send must fail")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("send did not stop promptly: %s", time.Since(start))
	}
}

func TestSendStopsAttachmentAfterSummaryAndNeverRetries(t *testing.T) {
	requests := 0
	failOnce := true
	ts := okServer(t, func(r *http.Request, body []byte) *string {
		requests++
		if strings.HasSuffix(r.URL.Path, "/sendDocument") || failOnce {
			// Both requests fail; there must be exactly one per method.
			failOnce = false
			reply := `{"ok":false}`
			return &reply
		}
		return nil
	})
	dir := t.TempDir()
	listing := filepath.Join(dir, "events.txt")
	if err := os.WriteFile(listing, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewSenderWithBase("t", "c", ts.URL)
	if err := s.Send(context.Background(), "text", listing); err == nil {
		t.Fatal("a failed summary must fail the send")
	}
	if requests != 1 {
		t.Fatalf("requests = %d: a failed summary must stop before the attachment", requests)
	}
}

func TestSendWithoutAttachmentSendsOneRequest(t *testing.T) {
	requests := 0
	ts := okServer(t, func(*http.Request, []byte) *string {
		requests++
		return nil
	})
	s := NewSenderWithBase("t", "c", ts.URL)
	if err := s.Send(context.Background(), "text", ""); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d", requests)
	}
}

func parseValues(r *http.Request, body []byte) (map[string]string, error) {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/") {
		_, params, err := mime.ParseMediaType(ct)
		if err != nil {
			return nil, err
		}
		mr := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
		form, err := mr.ReadForm(1 << 20)
		if err != nil {
			return nil, err
		}
		values := map[string]string{}
		for key, list := range form.Value {
			if len(list) > 0 {
				values[key] = list[0]
			}
		}
		return values, nil
	}
	parsed, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for key, list := range parsed {
		if len(list) > 0 {
			values[key] = list[0]
		}
	}
	return values, nil
}
