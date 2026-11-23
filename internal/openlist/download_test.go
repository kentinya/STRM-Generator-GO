package openlist

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// hijack extracts the raw connection of one test-server response writer. It
// exists only for the cut-mid-stream case, which the normal handler API cannot
// express.
func hijack(w http.ResponseWriter) (net.Conn, *bufio.ReadWriter, bool) {
	h, ok := w.(http.Hijacker)
	if !ok {
		return nil, nil, false
	}
	conn, rw, err := h.Hijack()
	if err != nil {
		return nil, nil, false
	}
	return conn, rw, true
}

func newTestDownloader(prefix string) *SubtitleDownloader {
	// The test client bypasses the production transport on purpose so the
	// transfer-contract tests exercise the fetch loop alone; the pacing and
	// protocol tests use the production constructor path instead.
	return NewSubtitleDownloader(prefix, 0, false).WithHTTPClient(&http.Client{})
}

func TestSubtitleFetchUsesStableAddressWithoutToken(t *testing.T) {
	var gotPath, gotAuth string
	var gotMethod string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotMethod = r.URL.Path, r.Header.Get("Authorization"), r.Method
		w.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"))
	}))
	defer ts.Close()
	d := newTestDownloader(ts.URL)
	var out strings.Builder
	if err := d.Fetch(context.Background(), "/Movies/中文 Sub.srt", &out); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotPath != "/d/Movies/中文 Sub.srt" {
		t.Fatalf("request path = %q, want the encoded /d/ address", gotPath)
	}
	if gotAuth != "" {
		t.Fatal("the download must not carry the OpenList token")
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q", gotMethod)
	}
	if out.String() == "" {
		t.Fatal("body was not delivered")
	}
}

func TestSubtitleFetchFollowsUpToFiveRedirects(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/d/Movies/a.srt" {
			http.Redirect(w, r, "/r1", http.StatusFound)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/r") {
			if n, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/r")); err == nil && n < 5 {
				http.Redirect(w, r, fmt.Sprintf("/r%d", n+1), http.StatusFound)
				return
			}
			w.Write([]byte("final\n"))
			return
		}
		t.Errorf("unexpected path %q", r.URL.Path)
	}))
	defer ts.Close()
	d := newTestDownloader(ts.URL)
	var out strings.Builder
	if err := d.Fetch(context.Background(), "/Movies/a.srt", &out); err != nil {
		t.Fatalf("fetch with five redirects: %v", err)
	}
	if out.String() != "final\n" {
		t.Fatalf("body = %q", out.String())
	}
}

func TestSubtitleFetchRejectsSixthRedirect(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer ts.Close()
	d := newTestDownloader(ts.URL)
	err := d.Fetch(context.Background(), "/Movies/a.srt", io.Discard)
	if !errors.Is(err, errSubtitleRedirects) {
		t.Fatalf("sixth redirect error = %v", err)
	}
}

func TestSubtitleFetchRejectsNonHTTPRedirect(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/d/Movies/a.srt" {
			w.Header().Set("Location", "ftp://example.invalid/x")
			w.WriteHeader(http.StatusFound)
			return
		}
		t.Errorf("unexpected path %q", r.URL.Path)
	}))
	defer ts.Close()
	d := newTestDownloader(ts.URL)
	err := d.Fetch(context.Background(), "/Movies/a.srt", io.Discard)
	if !errors.Is(err, errSubtitleRedirectType) {
		t.Fatalf("non-http redirect error = %v", err)
	}
}

func TestSubtitleFetchRejectsNon200Status(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer ts.Close()
	d := newTestDownloader(ts.URL)
	err := d.Fetch(context.Background(), "/Movies/a.srt", io.Discard)
	if !errors.Is(err, errSubtitleStatus) {
		t.Fatalf("404 error = %v", err)
	}
}

func TestSubtitleFetchRejectsHTMLOrJSONErrorPages(t *testing.T) {
	for name, contentType := range map[string]string{
		"html":        "text/html; charset=utf-8",
		"json":        "application/json",
		"json suffix": "application/problem+json",
	} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", contentType)
			w.Write([]byte("<html>404 not found</html>"))
		}))
		d := newTestDownloader(ts.URL)
		err := d.Fetch(context.Background(), "/Movies/a.srt", io.Discard)
		if !errors.Is(err, errSubtitleErrorPage) {
			t.Fatalf("%s error page error = %v", name, err)
		}
		ts.Close()
	}
}

func TestSubtitleFetchAcceptsPlainSubtitleContentTypes(t *testing.T) {
	for _, contentType := range []string{"text/plain", "application/octet-stream", "text/vtt", ""} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if contentType != "" {
				w.Header().Set("Content-Type", contentType)
			}
			w.Write([]byte("payload"))
		}))
		d := newTestDownloader(ts.URL)
		var out strings.Builder
		if err := d.Fetch(context.Background(), "/Movies/a.srt", &out); err != nil {
			t.Fatalf("content type %q: %v", contentType, err)
		}
		if out.String() != "payload" {
			t.Fatalf("content type %q body = %q", contentType, out.String())
		}
		ts.Close()
	}
}

func TestSubtitleFetchRejectsTruncatedBody(t *testing.T) {
	// A raw listener writes a 200 response that declares 100 bytes, delivers
	// ten and closes cleanly. The copy ends without a transport error, so the
	// declared-versus-actual byte check is the only remaining witness.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Consume the request head first: answering before the request is
		// drained makes the client see a reset while it still writes.
		head := make([]byte, 1)
		served := ""
		for !strings.Contains(served, "\r\n\r\n") {
			n, err := conn.Read(head)
			if err != nil {
				return
			}
			served += string(head[:n])
		}
		raw := "HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\nonly ten\n"
		_, _ = conn.Write([]byte(raw))
	}()
	d := newTestDownloader("http://" + listener.Addr().String())
	err = d.Fetch(context.Background(), "/Movies/a.srt", io.Discard)
	if !errors.Is(err, errSubtitleTruncated) {
		t.Fatalf("truncated error = %v", err)
	}
}

func TestSubtitleFetchRejectsBodyCutMidStream(t *testing.T) {
	// The Go test server aborts the connection after a short write, which
	// surfaces as a read error: still a download failure, never a success.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.(http.Flusher).Flush()
		conn, _, _ := hijack(w)
		w.Write([]byte("only ten\n"))
		conn.Close()
	}))
	defer ts.Close()
	d := newTestDownloader(ts.URL)
	err := d.Fetch(context.Background(), "/Movies/a.srt", io.Discard)
	if err == nil {
		t.Fatal("a cut-off body must fail the download")
	}
}

func TestSubtitleFetchRejectsBodyAboveLimitWithoutStoringIt(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, SubtitleMaxBytes+1024))
	}))
	defer ts.Close()
	d := newTestDownloader(ts.URL)
	var stored int64
	err := d.Fetch(context.Background(), "/Movies/a.srt", &countingSink{n: &stored})
	if !errors.Is(err, errSubtitleTooLarge) {
		t.Fatalf("oversized error = %v", err)
	}
	if stored > SubtitleMaxBytes+1 {
		t.Fatalf("writer stored %d bytes, %d above the limit", stored, stored-SubtitleMaxBytes)
	}
}

func TestSubtitleFetchRejectsDeclaredLengthAboveLimitBeforeReading(t *testing.T) {
	var served bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served = true
		w.Header().Set("Content-Length", "9000000")
		w.Write([]byte("x"))
	}))
	defer ts.Close()
	d := newTestDownloader(ts.URL)
	var stored int64
	err := d.Fetch(context.Background(), "/Movies/a.srt", &countingSink{n: &stored})
	if !errors.Is(err, errSubtitleTooLarge) {
		t.Fatalf("declared oversized error = %v", err)
	}
	if !served {
		t.Fatal("server was not consulted")
	}
	if stored != 0 {
		t.Fatalf("%d bytes were stored before the refusal", stored)
	}
}

func TestSubtitleFetchAcceptsBodyExactlyAtLimit(t *testing.T) {
	body := make([]byte, SubtitleMaxBytes)
	_, _ = rand.Read(body[:64])
	// The leading byte is fixed so the body can never look like an HTML or
	// JSON error page; the rest stays random for the limit check.
	body[0] = 'x'
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer ts.Close()
	d := newTestDownloader(ts.URL)
	var stored int64
	if err := d.Fetch(context.Background(), "/Movies/a.srt", &countingSink{n: &stored}); err != nil {
		t.Fatalf("exact-limit body: %v", err)
	}
	if stored != SubtitleMaxBytes {
		t.Fatalf("stored %d bytes, want %d", stored, SubtitleMaxBytes)
	}
}

func TestSubtitleFetchHonoursContextCancellation(t *testing.T) {
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.Write([]byte("late"))
	}))
	defer ts.Close()
	defer close(release)
	d := newTestDownloader(ts.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := d.Fetch(ctx, "/Movies/a.srt", io.Discard)
	if !errors.Is(err, errSubtitleTransport) {
		t.Fatalf("cancelled fetch error = %v", err)
	}
}

func TestSubtitleFetchRejectsInvalidRemotePath(t *testing.T) {
	d := newTestDownloader("https://openlist.example.invalid")
	for _, path := range []string{"Movies/a.srt", "/Movies/../a.srt", ""} {
		if err := d.Fetch(context.Background(), path, io.Discard); !errors.Is(err, errSubtitleURL) {
			t.Fatalf("path %q error = %v", path, err)
		}
	}
}

func TestSubtitleDownloadBudgetIsThirtySeconds(t *testing.T) {
	if SubtitleDownloadBudget != 30*time.Second {
		t.Fatalf("budget = %v", SubtitleDownloadBudget)
	}
	if SubtitleMaxRedirects != 5 {
		t.Fatalf("redirect limit = %d", SubtitleMaxRedirects)
	}
	if SubtitleMaxBytes != int64(8<<20) {
		t.Fatalf("byte limit = %d", SubtitleMaxBytes)
	}
}

// countingSink counts the bytes handed to it, mirroring what a real bounded
// temporary file would store.
type countingSink struct{ n *int64 }

func (c *countingSink) Write(p []byte) (int, error) {
	atomic.AddInt64(c.n, int64(len(p)))
	return len(p), nil
}

// TestSubtitleFetchRejectsErrorBodiesWithPlainContentType covers an OpenList
// error body answered with a harmless content type: the streamed bytes decide,
// so the caller can never publish an error page as a subtitle.
func TestSubtitleFetchRejectsErrorBodiesWithPlainContentType(t *testing.T) {
	longArray := "[" + strings.Repeat(`{"code":500},`, 80) + `{"code":500}]`
	cases := map[string]string{
		"json object":            `{"code":500,"message":"fixture error"}`,
		"json object indented":   "\n\n  {\"code\":500}",
		"json array":             `[{"code":500}]`,
		"json array past window": longArray,
		"html doctype":           "<!DOCTYPE html><html><body>502 bad gateway</body></html>",
	}
	for name, body := range cases {
		for _, contentType := range []string{"text/plain", "application/octet-stream", ""} {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if contentType != "" {
					w.Header().Set("Content-Type", contentType)
				}
				_, _ = w.Write([]byte(body))
			}))
			d := newTestDownloader(ts.URL)
			err := d.Fetch(context.Background(), "/Movies/a.srt", io.Discard)
			ts.Close()
			if !errors.Is(err, errSubtitleErrorPage) {
				t.Fatalf("%s (%q): error = %v, want an error page refusal", name, contentType, err)
			}
		}
	}
}

// TestSubtitleFetchAcceptsAssSectionHeader keeps the ambiguous '[' prefix
// honest: a legal ASS subtitle starts with a section header and must keep its
// exact bytes even when it is longer than the sniff window.
func TestSubtitleFetchAcceptsAssSectionHeader(t *testing.T) {
	body := "[Script Info]\nTitle: fixture\nScriptType: v4.00+\n\n[Events]\n"
	for len(body) < errorPageSniffBytes*2 {
		body += "Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,hello\n"
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()
	d := newTestDownloader(ts.URL)
	var out strings.Builder
	if err := d.Fetch(context.Background(), "/Movies/a.srt", &out); err != nil {
		t.Fatalf("ass body: %v", err)
	}
	if out.String() != body {
		t.Fatal("the accepted subtitle bytes changed")
	}
}

// gatedSink signals when the first body chunk reaches the destination and then
// holds the copy until the test releases it, so a cancellation lands inside
// the in-flight body read instead of before the request.
type gatedSink struct {
	arrived chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *gatedSink) Write(p []byte) (int, error) {
	g.once.Do(func() { close(g.arrived) })
	<-g.release
	return len(p), nil
}

// TestSubtitleFetchCancelDuringBodyRead cancels while response bytes are
// already streaming into the destination writer.
func TestSubtitleFetchCancelDuringBodyRead(t *testing.T) {
	started := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("first chunk of a subtitle"))
		// Flushing without a declared length keeps the body open as a
		// chunked stream until the handler returns.
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer ts.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gate := &gatedSink{arrived: make(chan struct{}), release: make(chan struct{})}
	d := newTestDownloader(ts.URL)
	errCh := make(chan error, 1)
	go func() { errCh <- d.Fetch(ctx, "/Movies/a.srt", gate) }()
	<-started
	<-gate.arrived
	cancel()
	close(gate.release)
	if err := <-errCh; !errors.Is(err, errSubtitleTransport) {
		t.Fatalf("cancelled body read error = %v", err)
	}
}

// TestSubtitleDownloadBudgetStopsStalledBody proves the thirty-second budget
// really ends a response body that never finishes, instead of existing only
// as a documented constant.
func TestSubtitleDownloadBudgetStopsStalledBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("stalled"))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(SubtitleDownloadBudget + 20*time.Second):
		}
	}))
	defer ts.Close()
	d := newTestDownloader(ts.URL)
	start := time.Now()
	err := d.Fetch(context.Background(), "/Movies/a.srt", io.Discard)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("a stalled body must fail the download")
	}
	if elapsed < SubtitleDownloadBudget-2*time.Second {
		t.Fatalf("failed after %v, before the %v budget", elapsed, SubtitleDownloadBudget)
	}
	if elapsed > SubtitleDownloadBudget+10*time.Second {
		t.Fatalf("failed after %v, the budget did not bound the body read", elapsed)
	}
}

// TestSubtitleFetchRejectsBOMErrorPages covers an error page saved with a
// UTF-8 byte order mark and answered with a harmless content type: the mark
// is skipped for classification, never stripped from stored bytes, so a BOM'd
// HTML or JSON body can never reach a published subtitle.
func TestSubtitleFetchRejectsBOMErrorPages(t *testing.T) {
	bom := string([]byte{0xEF, 0xBB, 0xBF})
	longArray := "[" + strings.Repeat(`{"code":500},`, 80) + `{"code":500}]`
	cases := map[string]string{
		"bom html":              bom + "<!DOCTYPE html><html><body>502 bad gateway</body></html>",
		"bom html after blank":  "\n \t" + bom + "<html>not found</html>",
		"bom json object":       bom + `{"code":500,"message":"fixture error"}`,
		"bom json array":        bom + `[{"code":500}]`,
		"bom json array window": bom + longArray,
	}
	for name, body := range cases {
		for _, contentType := range []string{"text/plain", "application/octet-stream", ""} {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if contentType != "" {
					w.Header().Set("Content-Type", contentType)
				}
				_, _ = w.Write([]byte(body))
			}))
			d := newTestDownloader(ts.URL)
			err := d.Fetch(context.Background(), "/Movies/a.srt", io.Discard)
			ts.Close()
			if !errors.Is(err, errSubtitleErrorPage) {
				t.Fatalf("%s (%q): error = %v, want an error page refusal", name, contentType, err)
			}
		}
	}
}

// TestSubtitleFetchRejectsBOMErrorPageSplitAcrossReads delivers the mark in
// its own flushed chunk, the way a chunked response can split it: the
// classification must wait for the bytes after the mark instead of settling
// on the first byte of an incomplete mark.
func TestSubtitleFetchRejectsBOMErrorPageSplitAcrossReads(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
		w.(http.Flusher).Flush()
		// Hold the rest long enough for the client to consume the first
		// chunk on its own, which is what splits the mark across reads.
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("<html>502 bad gateway</html>"))
	}))
	defer ts.Close()
	d := newTestDownloader(ts.URL)
	err := d.Fetch(context.Background(), "/Movies/a.srt", io.Discard)
	if !errors.Is(err, errSubtitleErrorPage) {
		t.Fatalf("split-mark error page error = %v", err)
	}
}

// TestSubtitleFetchAcceptsBOMSubtitles keeps the other side honest: a legal
// subtitle saved with a UTF-8 mark, including an ASS file whose section
// header shares the ambiguous '[' prefix, is published with its exact bytes.
func TestSubtitleFetchAcceptsBOMSubtitles(t *testing.T) {
	bom := []byte{0xEF, 0xBB, 0xBF}
	srt := append(append([]byte{}, bom...),
		[]byte("1\n00:00:01,000 --> 00:00:02,000\nbom track\n")...)
	ass := append(append([]byte{}, bom...), []byte("[Script Info]\nTitle: fixture\n")...)
	for len(ass) < errorPageSniffBytes*2 {
		ass = append(ass, []byte("Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,hello\n")...)
	}
	for name, body := range map[string][]byte{"srt": srt, "ass": ass} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write(body)
		}))
		d := newTestDownloader(ts.URL)
		var out strings.Builder
		err := d.Fetch(context.Background(), "/Movies/a.srt", &out)
		ts.Close()
		if err != nil {
			t.Fatalf("%s body: %v", name, err)
		}
		if out.String() != string(body) {
			t.Fatalf("%s bytes changed", name)
		}
	}
}

// fakeClock advances by a fixed step on every call, so the pacer math can be
// observed without real sleeps; the injected sleep records the computed delay
// instead of blocking.
type fakeClock struct {
	mu   sync.Mutex
	step time.Duration
	t    time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Unix(1000000, 0)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(c.step)
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// SUB-2: the first attempt never waits; a fast attempt inside the interval
// waits exactly the remaining time; an attempt that starts after the interval
// elapsed never waits again, so idle time never accumulates burst credit.
func TestSubtitlePacerScheduling(t *testing.T) {
	clock := newFakeClock()
	clock.step = 100 * time.Millisecond
	var sleeps []time.Duration
	p := &downloadPacer{interval: time.Second, now: clock.Now,
		sleep: func(ctx context.Context, delay time.Duration) error {
			sleeps = append(sleeps, delay)
			clock.advance(delay)
			return nil
		}}
	ctx := context.Background()
	if err := p.wait(ctx); err != nil {
		t.Fatalf("first wait: %v", err)
	}
	if len(sleeps) != 0 {
		t.Fatalf("the first attempt must start immediately: %v", sleeps)
	}
	first := p.last
	if err := p.wait(ctx); err != nil {
		t.Fatalf("second wait: %v", err)
	}
	if len(sleeps) != 1 || sleeps[0] != 900*time.Millisecond {
		t.Fatalf("second wait delay = %v, want 900ms", sleeps)
	}
	if !p.last.After(first) {
		t.Fatal("the recorded start must move to the actual start after the wait")
	}
	// A slow attempt whose elapsed time already covers the interval never
	// waits again and never owes an extra fixed wait.
	clock.advance(5 * time.Second)
	if err := p.wait(ctx); err != nil {
		t.Fatalf("slow attempt wait: %v", err)
	}
	if len(sleeps) != 1 {
		t.Fatalf("slow attempt must not wait: %v", sleeps)
	}
	// The attempt right after a zero-wait start still waits only the remaining
	// part of the interval: no debt, no burst, never more than one interval.
	if err := p.wait(ctx); err != nil {
		t.Fatalf("next attempt wait: %v", err)
	}
	if len(sleeps) != 2 || sleeps[1] != 900*time.Millisecond {
		t.Fatalf("next attempt delay = %v, want [900ms 900ms]", sleeps)
	}
	// The long idle above never accumulated burst credit: the delay after the
	// idle stayed the plain remaining interval, never more than one interval.
	for _, delay := range sleeps {
		if delay > p.interval {
			t.Fatalf("delay %v exceeds the interval", delay)
		}
	}
}

// SUB-2: a cancelled wait never records an attempt, so the next successful
// wait still spaces against the last real attempt.
func TestSubtitlePacerWaitCancellation(t *testing.T) {
	clock := newFakeClock()
	clock.step = time.Second
	cancelled := errors.New("cancelled")
	p := &downloadPacer{interval: 5 * time.Second, now: clock.Now,
		sleep: func(ctx context.Context, delay time.Duration) error { return cancelled }}
	if err := p.wait(context.Background()); err != nil {
		t.Fatalf("first wait: %v", err)
	}
	first := p.last
	clock.advance(2 * time.Second)
	if err := p.wait(context.Background()); err != cancelled {
		t.Fatalf("cancelled wait error = %v", err)
	}
	if !p.last.Equal(first) {
		t.Fatal("a cancelled wait must not record an attempt")
	}
	// A pre-cancelled context never reaches the sleep at all.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.wait(ctx); err == nil {
		t.Fatal("a pre-cancelled wait must fail")
	}
}

// SUB-2: the real production downloader spaces sequential fetches by the
// configured interval using real request timestamps, with no extra fixed wait
// after a slow attempt.
func TestSubtitleDownloaderRequestSpacing(t *testing.T) {
	var mu sync.Mutex
	var times []time.Time
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		w.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"))
	}))
	defer ts.Close()
	d := NewSubtitleDownloader(ts.URL, time.Second, false)
	for i := 0; i < 3; i++ {
		var out strings.Builder
		if err := d.Fetch(context.Background(), fmt.Sprintf("/Movies/Sub%d.srt", i), &out); err != nil {
			t.Fatalf("fetch %d: %v", i, err)
		}
	}
	if len(times) != 3 {
		t.Fatalf("request count = %d", len(times))
	}
	for i := 1; i < len(times); i++ {
		gap := times[i].Sub(times[i-1])
		// The handler runs after the attempt starts; a small scheduling skew
		// is tolerated, but an early request is never tolerated.
		if gap < 950*time.Millisecond {
			t.Fatalf("gap %d = %v, want at least ~1s", i, gap)
		}
	}
}

// SUB-2: one subtitle is in flight at a time by construction; the production
// constructor keeps the pacer round-shared. A fast sequence still honours the
// interval across separate Fetch calls on the same downloader.
func TestSubtitleDownloaderPacerSharedAcrossFetches(t *testing.T) {
	d := NewSubtitleDownloader("https://openlist.example.invalid", time.Second, false)
	if d.pacer == nil || d.pacer.interval != time.Second {
		t.Fatalf("pacer = %+v", d.pacer)
	}
	// A cancelled wait must be observable before any request is built.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out strings.Builder
	if err := d.Fetch(ctx, "/Movies/Sub.srt", &out); err == nil {
		t.Fatal("a cancelled round must not start the transfer")
	}
}

// SUB-3: an HTTPS test server that actually supports HTTP/2 records the
// negotiated protocol of every request. The production constructor with
// use_http2 disabled must negotiate HTTP/1.1, and the explicit true control
// must really negotiate HTTP/2 on the same server, so the assertion cannot
// pass just because the test server lacks the protocol.
func newHTTP2RecordingServer(t *testing.T, record func(proto string)) *httptest.Server {
	t.Helper()
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proto := r.Proto
		if r.TLS != nil && r.TLS.NegotiatedProtocol != "" {
			proto = r.TLS.NegotiatedProtocol
		}
		record(strings.ToLower(proto))
		w.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"))
	}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	t.Cleanup(ts.Close)
	return ts
}

// mustServerPool builds a root pool that trusts one test server's certificate.
func mustServerPool(t *testing.T, ts *httptest.Server) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(ts.Certificate())
	return pool
}

func TestSubtitleTransportDefaultsToHTTP1OverHTTPS(t *testing.T) {
	var protos []string
	var mu sync.Mutex
	ts := newHTTP2RecordingServer(t, func(proto string) {
		mu.Lock()
		protos = append(protos, proto)
		mu.Unlock()
	})
	pool := mustServerPool(t, ts)
	d := NewSubtitleDownloader(ts.URL, 0, false).WithRootCAs(pool)
	var out strings.Builder
	if err := d.Fetch(context.Background(), "/Movies/Movie.zh.srt", &out); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(protos) != 1 {
		t.Fatalf("request count = %d", len(protos))
	}
	if protos[0] != "http/1.1" {
		t.Fatalf("negotiated protocol = %q, want http/1.1", protos[0])
	}
}

func TestSubtitleTransportExplicitHTTP2NegotiatesH2(t *testing.T) {
	var protos []string
	var mu sync.Mutex
	ts := newHTTP2RecordingServer(t, func(proto string) {
		mu.Lock()
		protos = append(protos, proto)
		mu.Unlock()
	})
	// The control case: the same server really supports HTTP/2. The control
	// client only adds the test server's root certificate; everything else is
	// the shared default transport.
	control := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}
	control.Transport.(*http.Transport).TLSClientConfig = &tls.Config{RootCAs: mustServerPool(t, ts)}
	resp, err := control.Get(ts.URL + "/d/Movies/Movie.zh.srt")
	if err != nil {
		t.Fatalf("control request: %v", err)
	}
	resp.Body.Close()
	mu.Lock()
	if len(protos) != 1 || protos[0] != "h2" {
		have := append([]string(nil), protos...)
		mu.Unlock()
		t.Fatalf("control negotiation = %v, the test server must really support h2", have)
	}
	mu.Unlock()
	// The production path with use_http2: true negotiates h2 as well.
	protos = nil
	d := NewSubtitleDownloader(ts.URL, 0, true).WithRootCAs(mustServerPool(t, ts))
	var out strings.Builder
	if err := d.Fetch(context.Background(), "/Movies/Movie.zh.srt", &out); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(protos) != 1 || protos[0] != "h2" {
		t.Fatalf("explicit h2 negotiation = %v, want [h2]", protos)
	}
}

// SUB-3: the default transport stays isolated: the subtitle client keeps its
// own transport, never mutates the shared default, and every hop of a
// cross-domain HTTPS redirect chain uses the same HTTP/1.1-only client.
func TestSubtitleTransportIsolationAndRedirectChain(t *testing.T) {
	var protos []string
	var mu sync.Mutex
	record := func(proto string) {
		mu.Lock()
		protos = append(protos, proto)
		mu.Unlock()
	}
	target := newHTTP2RecordingServer(t, record)
	origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proto := r.Proto
		if r.TLS != nil && r.TLS.NegotiatedProtocol != "" {
			proto = r.TLS.NegotiatedProtocol
		}
		record(strings.ToLower(proto))
		http.Redirect(w, r, target.URL+"/d/Movies/Movie.zh.srt", http.StatusFound)
	}))
	origin.EnableHTTP2 = true
	origin.StartTLS()
	t.Cleanup(origin.Close)

	defaultTransport := http.DefaultTransport.(*http.Transport)

	d := NewSubtitleDownloader(origin.URL, 0, false).WithRootCAs(func() *x509.CertPool {
		pool := x509.NewCertPool()
		pool.AddCert(target.Certificate())
		pool.AddCert(origin.Certificate())
		return pool
	}())
	var out strings.Builder
	if err := d.Fetch(context.Background(), "/Movies/Movie.zh.srt", &out); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(protos) != 2 {
		t.Fatalf("hops = %d, want 2", len(protos))
	}
	for i, proto := range protos {
		if proto != "http/1.1" {
			t.Fatalf("hop %d negotiated %q, want http/1.1", i, proto)
		}
	}
	if defaultTransport.Protocols != nil {
		t.Fatal("the shared default transport protocol set must stay untouched")
	}
	if d.transport == defaultTransport {
		t.Fatal("the subtitle client must not share the default transport")
	}
}
