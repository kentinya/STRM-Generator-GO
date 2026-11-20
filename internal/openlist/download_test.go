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
