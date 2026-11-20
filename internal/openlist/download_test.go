package openlist

import (
	"bufio"
	"context"
	"crypto/rand"
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
	return NewSubtitleDownloader(prefix).WithHTTPClient(&http.Client{})
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
