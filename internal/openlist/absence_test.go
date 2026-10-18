package openlist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// absenceServer is a programmable OpenList API stub. It records every request
// with its raw body so tests can assert the exact request parameters (an
// explicit refresh:false on every list page) and the exact request sequence
// (root read, target read, no listing and no ancestor walk).
type absenceServer struct {
	mu       sync.Mutex
	requests []recordedRequest
	list     func(path string) (any, int, int)
	get      func(path string) (any, int, int)
}

type recordedRequest struct {
	Endpoint string
	Path     string
	// Body is the raw decoded request payload, so a test can distinguish an
	// explicit "refresh": false from an omitted field.
	Body map[string]any
	// RefreshPresent and Refresh are the parsed refresh field of a list request.
	RefreshPresent bool
	Refresh        bool
}

func (s *absenceServer) record(req recordedRequest) {
	s.mu.Lock()
	s.requests = append(s.requests, req)
	s.mu.Unlock()
}

func (s *absenceServer) listed() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []recordedRequest
	for _, r := range s.requests {
		if r.Endpoint == "/api/fs/list" {
			out = append(out, r)
		}
	}
	return out
}

func (s *absenceServer) gets() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []recordedRequest
	for _, r := range s.requests {
		if r.Endpoint == "/api/fs/get" {
			out = append(out, r)
		}
	}
	return out
}

func (s *absenceServer) allRequests() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedRequest(nil), s.requests...)
}

func (s *absenceServer) serve() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/fs/list", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Path    string `json:"path"`
			Page    int    `json:"page"`
			PerPage int    `json:"per_page"`
			Refresh *bool  `json:"refresh"`
		}
		json.Unmarshal(body, &req)
		recorded := recordedRequest{Endpoint: "/api/fs/list", Path: req.Path, Body: map[string]any{}}
		json.Unmarshal(body, &recorded.Body)
		if req.Refresh != nil {
			recorded.RefreshPresent = true
			recorded.Refresh = *req.Refresh
		}
		s.record(recorded)
		s.mu.Lock()
		handler := s.list
		s.mu.Unlock()
		if handler == nil {
			writeEnvelope(w, 200, 500, nil)
			return
		}
		data, status, code := handler(req.Path)
		writeEnvelope(w, status, code, data)
	})
	mux.HandleFunc("/api/fs/get", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct{ Path string }
		json.Unmarshal(body, &req)
		recorded := recordedRequest{Endpoint: "/api/fs/get", Path: req.Path, Body: map[string]any{}}
		json.Unmarshal(body, &recorded.Body)
		s.record(recorded)
		s.mu.Lock()
		handler := s.get
		s.mu.Unlock()
		if handler == nil {
			writeEnvelope(w, 200, 500, nil)
			return
		}
		data, status, code := handler(req.Path)
		writeEnvelope(w, status, code, data)
	})
	return httptest.NewServer(mux)
}

// node is one synthetic remote object. A directory answers a Get with
// is_dir:true; a file answers with is_dir:false.
type node struct {
	isDir bool
}

// treeServer builds an absenceServer over a path -> node map. A path that is
// not in the tree answers with the pinned build's shape for a missing object:
// HTTP 200 with application code 500.
func treeServer(tree map[string]node) *absenceServer {
	return &absenceServer{
		list: func(path string) (any, int, int) {
			names := make([]string, 0, len(tree))
			prefix := path + "/"
			for candidate := range tree {
				if !strings.HasPrefix(candidate, prefix) {
					continue
				}
				rest := strings.TrimPrefix(candidate, prefix)
				if strings.Contains(rest, "/") || rest == "" {
					continue
				}
				names = append(names, rest)
			}
			content := make([]map[string]any, 0, len(names))
			for _, name := range names {
				content = append(content, map[string]any{
					"name": name, "is_dir": tree[remotePathJoin(path, name)].isDir, "size": 0,
				})
			}
			if len(content) == 0 {
				return map[string]any{"content": nil, "total": 0}, 200, 200
			}
			return map[string]any{"content": content, "total": len(content)}, 200, 200
		},
		get: func(path string) (any, int, int) {
			n, ok := tree[path]
			if !ok {
				return nil, 200, 500
			}
			name := path
			if idx := strings.LastIndex(path, "/"); idx >= 0 {
				name = path[idx+1:]
			}
			if path == "/" {
				name = "root"
			}
			return map[string]any{"name": name, "is_dir": n.isDir, "size": 0, "provider": "Local"}, 200, 200
		},
	}
}

func newTreeConfirmer(t *testing.T, tree map[string]node) (*Confirmer, *absenceServer) {
	t.Helper()
	srv := treeServer(tree)
	ts := srv.serve()
	t.Cleanup(ts.Close)
	return NewConfirmer(New(ts.URL, "token")), srv
}

func remotePathJoin(dir, name string) string {
	if dir == "/" {
		return "/" + name
	}
	return dir + "/" + name
}

func dirNode() node { return node{isDir: true} }
func fileNode() node {
	return node{}
}

// getPaths returns the exact paths read through Get, in request order.
func getPaths(srv *absenceServer) []string {
	gets := srv.gets()
	out := make([]string, 0, len(gets))
	for _, req := range gets {
		out = append(out, req.Path)
	}
	return out
}

func TestConfirmReadsExactlyRootAndTarget(t *testing.T) {
	tree := map[string]node{
		"/Root":              dirNode(),
		"/Root/Movies":       dirNode(),
		"/Root/Movies/A.mkv": fileNode(),
	}
	c, srv := newTreeConfirmer(t, tree)
	ctx := context.Background()

	if got := c.Confirm(ctx, "/Root", "/Root/Movies/A.mkv"); got.Status != AbsencePresent || got.Reason != ReasonTargetPresent {
		t.Fatalf("existing file: %+v", got)
	}
	if got := c.Confirm(ctx, "/Root", "/Root/Movies/Missing.mkv"); got.Status != AbsenceMissing || got.Reason != ReasonTargetUnread {
		t.Fatalf("missing file: %+v", got)
	}
	if got := c.Confirm(ctx, "/Root", "/Root/Movies"); got.Status != AbsencePresent || got.Reason != ReasonTargetTypeConflict {
		t.Fatalf("directory target must be present with a type conflict: %+v", got)
	}
	if n := len(srv.listed()); n != 0 {
		t.Fatalf("confirmation must not list a directory, got %d list requests", n)
	}
	if got := getPaths(srv); len(got) != 3*MaxAbsenceReads {
		t.Fatalf("expected exactly %d reads per call, got %v", MaxAbsenceReads, got)
	}
}

func TestConfirmExactNamesAndSimilarPrefixes(t *testing.T) {
	tree := map[string]node{
		"/Root":               dirNode(),
		"/Root/Movies":        dirNode(),
		"/Root/Movies/A.mkv":  fileNode(),
		"/Root/Movies2":       dirNode(),
		"/Root/movies":        dirNode(),
		"/Root/中文 名字.mkv":     fileNode(),
		"/Root/#hash% &quo":   fileNode(),
		"/Root/#hash% &quote": fileNode(),
	}
	c, _ := newTreeConfirmer(t, tree)
	ctx := context.Background()

	for _, tc := range []struct {
		target string
		want   AbsenceStatus
	}{
		{"/Root/Movies/A.mkv", AbsencePresent},
		{"/Root/Movies/a.mkv", AbsenceMissing},
		{"/Root/Movies/A.MKV", AbsenceMissing},
		{"/Root/Movies2", AbsencePresent},
		{"/Root/Movie", AbsenceMissing},
		{"/Root/movies", AbsencePresent},
		{"/Root/中文 名字.mkv", AbsencePresent},
		{"/Root/#hash% &quote", AbsencePresent},
		{"/Root/#hash% &quo", AbsencePresent},
		{"/Root/#hash% &quoted", AbsenceMissing},
		{"/Root/Movies/A.mkv2", AbsenceMissing},
	} {
		if got := c.Confirm(ctx, "/Root", tc.target); got.Status != tc.want {
			t.Fatalf("%s: got %+v want %s", tc.target, got, tc.want)
		}
	}
}

func TestConfirmRootSlash(t *testing.T) {
	tree := map[string]node{
		"/":             dirNode(),
		"/Movies":       dirNode(),
		"/Movies/A.mkv": fileNode(),
	}
	c, _ := newTreeConfirmer(t, tree)
	ctx := context.Background()
	if got := c.Confirm(ctx, "/", "/Movies/A.mkv"); got.Status != AbsencePresent {
		t.Fatalf("root-relative present: %+v", got)
	}
	if got := c.Confirm(ctx, "/", "/Movies/B.mkv"); got.Status != AbsenceMissing {
		t.Fatalf("root-relative missing: %+v", got)
	}
	if got := c.Confirm(ctx, "/", "/Movies"); got.Status != AbsencePresent {
		t.Fatalf("root-relative directory: %+v", got)
	}
}

func TestConfirmRootGateFailureDoesNotReadTarget(t *testing.T) {
	// Every root outcome that is not a successfully read directory must keep the
	// source and must not consume a target read.
	cases := []struct {
		name string
		get  func(path string) (any, int, int)
	}{
		{
			name: "missing object application 500",
			get:  func(string) (any, int, int) { return nil, 200, 500 },
		},
		{
			name: "forbidden application 403",
			get:  func(string) (any, int, int) { return nil, 200, 403 },
		},
		{
			name: "http 503",
			get:  func(string) (any, int, int) { return nil, 503, 503 },
		},
		{
			name: "http 404",
			get:  func(string) (any, int, int) { return nil, 404, 404 },
		},
		{
			name: "malformed json",
			get: func(string) (any, int, int) {
				// A data payload that is not an object cannot be decoded.
				return []any{"not", "an", "object"}, 200, 200
			},
		},
		{
			name: "missing name",
			get:  func(string) (any, int, int) { return map[string]any{"is_dir": true, "size": 0}, 200, 200 },
		},
		{
			name: "missing is_dir",
			get:  func(string) (any, int, int) { return map[string]any{"name": "Root", "size": 0}, 200, 200 },
		},
		{
			name: "root is a file",
			get: func(string) (any, int, int) {
				return map[string]any{"name": "Root", "is_dir": false, "size": 0}, 200, 200
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := &absenceServer{
				get: func(path string) (any, int, int) {
					if path != "/Root" {
						t.Fatalf("a failed root gate read %q", path)
					}
					return tc.get(path)
				},
			}
			ts := srv.serve()
			defer ts.Close()
			got := NewConfirmer(New(ts.URL, "t")).Confirm(context.Background(), "/Root", "/Root/Sub/A.mkv")
			if got.Status != AbsenceUnknown || got.Reason != ReasonRootUnavailable {
				t.Fatalf("%s must keep the source: %+v", tc.name, got)
			}
			if paths := getPaths(srv); len(paths) != 1 || paths[0] != "/Root" {
				t.Fatalf("only the source root may be read, got %v", paths)
			}
			if n := len(srv.listed()); n != 0 {
				t.Fatalf("root gate must not list, got %d", n)
			}
		})
	}
}

func TestConfirmRootTransportFailureKeepsSource(t *testing.T) {
	// The connection is closed before the request completes: a transport
	// failure at the root stage is a root gate failure, never a missing target.
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := ts.URL
	ts.Close()
	got := NewConfirmer(New(url, "t")).Confirm(context.Background(), "/Root", "/Root/A.mkv")
	if got.Status != AbsenceUnknown || got.Reason != ReasonRootUnavailable {
		t.Fatalf("transport failure at the root must keep the source: %+v", got)
	}
}

func TestConfirmRootSingleRequestTimeoutKeepsSource(t *testing.T) {
	// The client's own per-request timeout ends one request while the caller's
	// context stays valid. At the root stage that is a root gate failure.
	srv := &absenceServer{
		get: func(string) (any, int, int) {
			time.Sleep(200 * time.Millisecond)
			return map[string]any{"name": "Root", "is_dir": true, "size": 0}, 200, 200
		},
	}
	ts := srv.serve()
	defer ts.Close()
	c := NewConfirmer(New(ts.URL, "t").WithHTTPClient(&http.Client{Timeout: 20 * time.Millisecond}))
	got := c.Confirm(context.Background(), "/Root", "/Root/A.mkv")
	if got.Status != AbsenceUnknown || got.Reason != ReasonRootUnavailable {
		t.Fatalf("a single request timeout at the root must keep the source: %+v", got)
	}
	if paths := getPaths(srv); len(paths) != 1 {
		t.Fatalf("the target must not be read after a root timeout: %v", paths)
	}
}

func TestConfirmRootAcceptsReadableDirectoryRegardlessOfProvider(t *testing.T) {
	// Provider "unknown" marks a directory OpenList serves only because a nested
	// mount exists below it. The accepted rule adds no virtual-view condition:
	// a successfully read directory continues, and a nested-mount view is one of
	// the documented limitations.
	for _, provider := range []string{"Local", "unknown", ""} {
		srv := &absenceServer{
			get: func(path string) (any, int, int) {
				if path == "/Root" {
					return map[string]any{"name": "Root", "is_dir": true, "size": 0, "provider": provider}, 200, 200
				}
				return nil, 200, 500
			},
		}
		ts := srv.serve()
		got := NewConfirmer(New(ts.URL, "t")).Confirm(context.Background(), "/Root", "/Root/Movies/A.mkv")
		ts.Close()
		if got.Status != AbsenceMissing {
			t.Fatalf("provider %q: a readable root must continue to the target: %+v", provider, got)
		}
	}
}

func TestConfirmTargetNotReadIsMissingRegardlessOfCause(t *testing.T) {
	// The accepted rule records one missing observation for every target that is
	// not read, whatever the reason. The test still asserts each case reaches
	// the target read exactly once and never lists.
	cases := []struct {
		name string
		get  func(path string) (any, int, int)
	}{
		{
			name: "application 500 missing object",
			get:  func(string) (any, int, int) { return nil, 200, 500 },
		},
		{
			name: "application 403 permission filtered",
			get:  func(string) (any, int, int) { return nil, 200, 403 },
		},
		{
			name: "application 404",
			get:  func(string) (any, int, int) { return nil, 200, 404 },
		},
		{
			name: "http 404",
			get:  func(string) (any, int, int) { return nil, 404, 404 },
		},
		{
			name: "http 500",
			get:  func(string) (any, int, int) { return nil, 500, 500 },
		},
		{
			name: "http 503 driver failure",
			get:  func(string) (any, int, int) { return nil, 503, 503 },
		},
		{
			name: "invalid json",
			get:  func(string) (any, int, int) { return []any{"not", "an", "object"}, 200, 200 },
		},
		{
			name: "missing name",
			get:  func(string) (any, int, int) { return map[string]any{"is_dir": false, "size": 1}, 200, 200 },
		},
		{
			name: "missing is_dir",
			get:  func(string) (any, int, int) { return map[string]any{"name": "A.mkv", "size": 1}, 200, 200 },
		},
		{
			name: "different object name",
			get:  func(string) (any, int, int) { return map[string]any{"name": "Other.mkv", "is_dir": false}, 200, 200 },
		},
		{
			name: "empty object",
			get:  func(string) (any, int, int) { return map[string]any{}, 200, 200 },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := &absenceServer{
				get: func(path string) (any, int, int) {
					if path == "/Root" {
						return map[string]any{"name": "Root", "is_dir": true, "size": 0}, 200, 200
					}
					return tc.get(path)
				},
			}
			ts := srv.serve()
			defer ts.Close()
			got := NewConfirmer(New(ts.URL, "t")).Confirm(context.Background(), "/Root", "/Root/Movies/A.mkv")
			if got.Status != AbsenceMissing {
				t.Fatalf("%s must be one missing observation: %+v", tc.name, got)
			}
			if paths := getPaths(srv); len(paths) != 2 || paths[1] != "/Root/Movies/A.mkv" {
				t.Fatalf("%s: unexpected reads %v", tc.name, paths)
			}
			if n := len(srv.listed()); n != 0 {
				t.Fatalf("%s: must not list, got %d", tc.name, n)
			}
		})
	}
}

func TestConfirmTargetPresentTypes(t *testing.T) {
	srv := &absenceServer{
		get: func(path string) (any, int, int) {
			switch path {
			case "/Root":
				return map[string]any{"name": "Root", "is_dir": true, "size": 0}, 200, 200
			case "/Root/A.mkv":
				return map[string]any{"name": "A.mkv", "is_dir": false, "size": 12}, 200, 200
			case "/Root/Dir":
				return map[string]any{"name": "Dir", "is_dir": true, "size": 0}, 200, 200
			default:
				return nil, 200, 500
			}
		},
	}
	ts := srv.serve()
	defer ts.Close()
	c := NewConfirmer(New(ts.URL, "t"))
	ctx := context.Background()
	if got := c.Confirm(ctx, "/Root", "/Root/A.mkv"); got.Status != AbsencePresent {
		t.Fatalf("file: %+v", got)
	}
	// A directory at the exact target is a type conflict the caller must
	// preserve and report, never a missing observation.
	got := c.Confirm(ctx, "/Root", "/Root/Dir")
	if got.Status != AbsencePresent || got.Reason != ReasonTargetTypeConflict {
		t.Fatalf("directory: %+v", got)
	}
}

func TestConfirmTargetSingleRequestTimeoutIsMissing(t *testing.T) {
	// A single request timeout of the HTTP client leaves the caller context
	// valid. At the target stage the accepted rule records it as not read.
	srv := &absenceServer{
		get: func(path string) (any, int, int) {
			if path == "/Root" {
				return map[string]any{"name": "Root", "is_dir": true, "size": 0}, 200, 200
			}
			time.Sleep(200 * time.Millisecond)
			return map[string]any{"name": "A.mkv", "is_dir": false, "size": 1}, 200, 200
		},
	}
	ts := srv.serve()
	defer ts.Close()
	c := NewConfirmer(New(ts.URL, "t").WithHTTPClient(&http.Client{Timeout: 20 * time.Millisecond}))
	got := c.Confirm(context.Background(), "/Root", "/Root/A.mkv")
	if got.Status != AbsenceMissing || got.Reason != ReasonTargetUnread {
		t.Fatalf("a single target timeout is one missing observation: %+v", got)
	}
}

func TestConfirmDeadServiceKeepsSource(t *testing.T) {
	// A service that is completely gone fails the root read, so the source is
	// preserved. The target-stage transport failure is covered by the single
	// request timeout case above and by the real-service runner.
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := ts.URL
	ts.Close()
	if got := NewConfirmer(New(url, "t")).Confirm(context.Background(), "/Root", "/Root/A.mkv"); got.Status != AbsenceUnknown {
		t.Fatalf("a dead service must keep the source: %+v", got)
	}
}

// scriptedGetter is an in-process Getter for tests that need to control the
// exact ordering between a returning read and the caller's cancellation.
type scriptedGetter struct {
	get func(ctx context.Context, path string) (*Entry, error)
}

func (g scriptedGetter) Get(ctx context.Context, path string) (*Entry, error) {
	return g.get(ctx, path)
}

func TestConfirmCallerCancellationWinsOverResult(t *testing.T) {
	// The caller's cancellation or deadline aborts the whole call. It is
	// checked after each read returns too, so a response that arrived together
	// with the cancellation can never become a missing observation.
	t.Run("before any request", func(t *testing.T) {
		var reads atomic.Int64
		c := NewConfirmer(scriptedGetter{get: func(context.Context, string) (*Entry, error) {
			reads.Add(1)
			return &Entry{Name: "Root", IsDir: true}, nil
		}})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		got := c.Confirm(ctx, "/Root", "/Root/A.mkv")
		if got.Status != AbsenceUnknown || got.Reason != ReasonCancelled {
			t.Fatalf("cancelled call: %+v", got)
		}
		if n := reads.Load(); n != 0 {
			t.Fatalf("a cancelled call must not read, got %d reads", n)
		}
	})

	t.Run("root read returns after cancellation", func(t *testing.T) {
		var paths []string
		var mu sync.Mutex
		c := NewConfirmer(scriptedGetter{get: func(ctx context.Context, path string) (*Entry, error) {
			mu.Lock()
			paths = append(paths, path)
			mu.Unlock()
			if path == "/Root" {
				// The read completes successfully, but the caller cancelled while
				// it was in flight: the abort must win over the result.
				cancelAfterRead()
				return &Entry{Name: "Root", IsDir: true}, nil
			}
			return nil, errors.New("target must not be read")
		}})
		got := c.Confirm(shortLivedContext(t), "/Root", "/Root/A.mkv")
		if got.Status != AbsenceUnknown || got.Reason != ReasonCancelled {
			t.Fatalf("root-stage cancellation: %+v", got)
		}
		if len(paths) != 1 || paths[0] != "/Root" {
			t.Fatalf("only the root may be read: %v", paths)
		}
	})

	t.Run("target read returns a failure after cancellation", func(t *testing.T) {
		// The regression the previous candidate failed: the final read's failure
		// was classified even though the caller had already cancelled.
		c := NewConfirmer(scriptedGetter{get: func(ctx context.Context, path string) (*Entry, error) {
			if path == "/Root" {
				return &Entry{Name: "Root", IsDir: true}, nil
			}
			cancelAfterRead()
			return nil, &APIError{Op: "/api/fs/get", Detail: "application code 500", Code: 500}
		}})
		got := c.Confirm(shortLivedContext(t), "/Root", "/Root/A.mkv")
		if got.Status != AbsenceUnknown || got.Reason != ReasonCancelled {
			t.Fatalf("target-stage cancellation must not be a missing observation: %+v", got)
		}
	})

	t.Run("target read returns an object after cancellation", func(t *testing.T) {
		// Symmetrically, a success that races with cancellation is not present.
		c := NewConfirmer(scriptedGetter{get: func(ctx context.Context, path string) (*Entry, error) {
			if path == "/Root" {
				return &Entry{Name: "Root", IsDir: true}, nil
			}
			cancelAfterRead()
			return &Entry{Name: "A.mkv"}, nil
		}})
		got := c.Confirm(shortLivedContext(t), "/Root", "/Root/A.mkv")
		if got.Status != AbsenceUnknown || got.Reason != ReasonCancelled {
			t.Fatalf("a success racing with cancellation must still abort: %+v", got)
		}
	})

	t.Run("deadline expires during target read", func(t *testing.T) {
		c := NewConfirmer(scriptedGetter{get: func(ctx context.Context, path string) (*Entry, error) {
			if path == "/Root" {
				return &Entry{Name: "Root", IsDir: true}, nil
			}
			select {
			case <-ctx.Done():
				return &Entry{Name: "A.mkv"}, nil
			case <-time.After(2 * time.Second):
				return nil, errors.New("deadline was not delivered")
			}
		}})
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		got := c.Confirm(ctx, "/Root", "/Root/A.mkv")
		if got.Status != AbsenceUnknown || got.Reason != ReasonCancelled {
			t.Fatalf("an expired deadline must abort the call: %+v", got)
		}
	})
}

// shortLivedContext returns a caller context whose cancel function is reachable
// from the scripted getter, so a test can cancel exactly while a read is in
// flight instead of relying on a timing sleep.
//
// cancelAfterRead is only written before Confirm starts, in the same call, so
// no other test can observe a stale value.
var cancelAfterRead = func() {}

func shortLivedContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cancelAfterRead = cancel
	return ctx
}

func TestConfirmRejectsInvalidPathsBeforeAnyRequest(t *testing.T) {
	cases := []struct {
		root, target string
		reason       AbsenceReason
	}{
		{"/Root", "/Root", ReasonRootIsTarget},
		{"/Root", "/Other/A.mkv", ReasonInvalidInput},
		{"/Root", "/Rooted/A.mkv", ReasonInvalidInput},
		{"/Root", "/Root/../A.mkv", ReasonInvalidInput},
		{"/Root", "/Root//A.mkv", ReasonInvalidInput},
		{"/Root", "/Root/./A.mkv", ReasonInvalidInput},
		{"/Root", "/Root/A.mkv/", ReasonInvalidInput},
		{"/Root", "Root/A.mkv", ReasonInvalidInput},
		{"/Root", "/Root/A\\B.mkv", ReasonInvalidInput},
		{"/Root", "/Root\x00/A.mkv", ReasonInvalidInput},
		{"/Root", "/Root/\x7f/A.mkv", ReasonInvalidInput},
		{"/Root", "/Root/" + strings.Repeat("a", 256), ReasonInvalidInput},
		{"", "/Root/A.mkv", ReasonInvalidInput},
		{"/Root", "", ReasonInvalidInput},
		{"Root", "/Root/A.mkv", ReasonInvalidInput},
		{"/", "/", ReasonRootIsTarget},
	}
	for _, tc := range cases {
		srv := &absenceServer{get: func(string) (any, int, int) {
			return map[string]any{"name": "x", "is_dir": true, "size": 0}, 200, 200
		}}
		ts := srv.serve()
		c := NewConfirmer(New(ts.URL, "t"))
		got := c.Confirm(context.Background(), tc.root, tc.target)
		ts.Close()
		if got.Status != AbsenceUnknown || got.Reason != tc.reason {
			t.Fatalf("root=%q target=%q: got %+v, want unknown/%s", tc.root, tc.target, got, tc.reason)
		}
		if n := len(srv.allRequests()); n != 0 {
			t.Fatalf("root=%q target=%q issued %d requests", tc.root, tc.target, n)
		}
	}
}

func TestConfirmDepthLimit(t *testing.T) {
	segments := make([]string, MaxAbsenceDepth+1)
	for i := range segments {
		segments[i] = fmt.Sprintf("d%d", i)
	}
	target := "/Root/" + strings.Join(segments, "/")
	srv := &absenceServer{get: func(path string) (any, int, int) {
		name := path[strings.LastIndex(path, "/")+1:]
		return map[string]any{"name": name, "is_dir": true, "size": 0}, 200, 200
	}}
	ts := srv.serve()
	defer ts.Close()
	got := NewConfirmer(New(ts.URL, "t")).Confirm(context.Background(), "/Root", target)
	if got.Status != AbsenceUnknown || got.Reason != ReasonDepthLimit {
		t.Fatalf("depth limit: %+v", got)
	}
	if n := len(srv.allRequests()); n != 0 {
		t.Fatalf("depth limit must be rejected before any request, got %d", n)
	}

	// A path at the limit is still accepted and read exactly twice.
	exact := "/Root/" + strings.Join(segments[:MaxAbsenceDepth], "/")
	if got := NewConfirmer(New(ts.URL, "t")).Confirm(context.Background(), "/Root", exact); got.Status != AbsencePresent {
		t.Fatalf("path at the depth limit: %+v", got)
	}
}

func TestConfirmZeroValuesAreNotMissing(t *testing.T) {
	var zero AbsenceResult
	if zero.Status != AbsenceUnknown || zero.Status.String() != "unknown" {
		t.Fatalf("zero result must mean unknown: %+v", zero)
	}
	var nilConfirmer *Confirmer
	if got := nilConfirmer.Confirm(context.Background(), "/Root", "/Root/A.mkv"); got.Status != AbsenceUnknown {
		t.Fatalf("nil confirmer: %+v", got)
	}
	if got := NewConfirmer(nil).Confirm(context.Background(), "/Root", "/Root/A.mkv"); got.Status != AbsenceUnknown || got.Reason != ReasonDependencyMissing {
		t.Fatalf("nil reader: %+v", got)
	}
	var nilContext context.Context
	if got := NewConfirmer(staticGetter{}).Confirm(nilContext, "/Root", "/Root/A.mkv"); got.Status != AbsenceUnknown || got.Reason != ReasonCancelled {
		t.Fatalf("nil context: %+v", got)
	}
}

type staticGetter struct{}

func (staticGetter) Get(context.Context, string) (*Entry, error) { return nil, errors.New("unused") }

func TestConfirmReasonHasNoSecrets(t *testing.T) {
	srv := &absenceServer{get: func(path string) (any, int, int) {
		if path == "/Root" {
			return map[string]any{"name": "Root", "is_dir": true, "size": 0}, 200, 200
		}
		return nil, 200, 500
	}}
	ts := srv.serve()
	defer ts.Close()
	got := NewConfirmer(New(ts.URL, "super-secret-token")).Confirm(context.Background(), "/Root", "/Root/A.mkv")
	text := string(got.Reason)
	for _, banned := range []string{"super-secret-token", "http://", "APIError", "failed to get", "/Root"} {
		if strings.Contains(text, banned) {
			t.Fatalf("reason leaked %q: %q", banned, text)
		}
	}
	if strings.Contains(fmt.Sprintf("%+v", got), "super-secret-token") {
		t.Fatalf("result leaked the token: %+v", got)
	}
}

func TestConfirmReadsCurrentFactsOnEveryCall(t *testing.T) {
	var present atomic.Bool
	srv := &absenceServer{get: func(path string) (any, int, int) {
		if path == "/Root" {
			return map[string]any{"name": "Root", "is_dir": true, "size": 0}, 200, 200
		}
		if present.Load() {
			return map[string]any{"name": "A.mkv", "is_dir": false, "size": 1}, 200, 200
		}
		return nil, 200, 500
	}}
	ts := srv.serve()
	defer ts.Close()
	c := NewConfirmer(New(ts.URL, "t"))
	ctx := context.Background()
	if got := c.Confirm(ctx, "/Root", "/Root/A.mkv"); got.Status != AbsenceMissing {
		t.Fatalf("initial missing observation: %+v", got)
	}
	present.Store(true)
	if got := c.Confirm(ctx, "/Root", "/Root/A.mkv"); got.Status != AbsencePresent {
		t.Fatalf("a target that reappears must be read as present: %+v", got)
	}
	present.Store(false)
	if got := c.Confirm(ctx, "/Root", "/Root/A.mkv"); got.Status != AbsenceMissing {
		t.Fatalf("a target that disappears again must be a missing observation: %+v", got)
	}
}

func TestConfirmConcurrentCallsKeepIndependentState(t *testing.T) {
	const workers = 8
	srv := &absenceServer{get: func(path string) (any, int, int) {
		if path == "/Root" {
			return map[string]any{"name": "Root", "is_dir": true, "size": 0}, 200, 200
		}
		if strings.HasPrefix(path, "/Root/Present") {
			name := path[strings.LastIndex(path, "/")+1:]
			return map[string]any{"name": name, "is_dir": false, "size": 1}, 200, 200
		}
		return nil, 200, 500
	}}
	ts := srv.serve()
	defer ts.Close()
	c := NewConfirmer(New(ts.URL, "t"))
	var wg sync.WaitGroup
	results := make([]AbsenceResult, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("/Root/Present%d.mkv", i)
			if i%2 == 1 {
				name = fmt.Sprintf("/Root/Absent%d.mkv", i)
			}
			results[i] = c.Confirm(context.Background(), "/Root", name)
		}(i)
	}
	wg.Wait()
	for i, got := range results {
		if i%2 == 0 && got.Status != AbsencePresent {
			t.Fatalf("worker %d: %+v", i, got)
		}
		if i%2 == 1 && got.Status != AbsenceMissing {
			t.Fatalf("worker %d: %+v", i, got)
		}
	}
	if gets := srv.gets(); len(gets) != MaxAbsenceReads*workers {
		t.Fatalf("expected %d reads per call, got %d for %d calls", MaxAbsenceReads, len(gets), workers)
	}
}

func TestListAlwaysRequestsCachedPages(t *testing.T) {
	// Every list page must carry an explicit refresh:false. The key must be
	// present, not merely default to false, because the contract requires the
	// generator never to ask for a server-side refresh.
	all := make([]map[string]any, 0, perPage+1)
	for i := 0; i < perPage+1; i++ {
		all = append(all, map[string]any{"name": fmt.Sprintf("F%03d.mkv", i), "is_dir": false, "size": 0})
	}
	srv := &absenceServer{}
	srv.list = func(path string) (any, int, int) {
		page := 1
		for _, r := range srv.allRequests() {
			if r.Endpoint == "/api/fs/list" {
				page = int(r.Body["page"].(float64))
			}
		}
		start := (page - 1) * perPage
		end := start + perPage
		if start > len(all) {
			start = len(all)
		}
		if end > len(all) {
			end = len(all)
		}
		return map[string]any{"content": all[start:end], "total": len(all)}, 200, 200
	}
	ts := srv.serve()
	defer ts.Close()
	entries, err := New(ts.URL, "t").List(context.Background(), "/Movies")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != perPage+1 {
		t.Fatalf("got %d entries", len(entries))
	}
	listed := srv.listed()
	if len(listed) != 2 {
		t.Fatalf("expected two pages, got %d", len(listed))
	}
	for _, req := range listed {
		if !req.RefreshPresent {
			t.Fatalf("page %v omitted the refresh field: %+v", req.Body["page"], req.Body)
		}
		if req.Refresh {
			t.Fatalf("page %v requested a refresh", req.Body["page"])
		}
	}
}

func TestListAcceptsRealEmptyDirectoryShape(t *testing.T) {
	srv := &absenceServer{list: func(string) (any, int, int) {
		return map[string]any{"content": nil, "total": 0}, 200, 200
	}}
	ts := srv.serve()
	defer ts.Close()
	entries, err := New(ts.URL, "t").List(context.Background(), "/Movies")
	if err != nil || len(entries) != 0 {
		t.Fatalf("real empty directory: entries=%v err=%v", entries, err)
	}
	for _, req := range srv.listed() {
		if !req.RefreshPresent || req.Refresh {
			t.Fatalf("empty-directory list must still be refresh:false: %+v", req.Body)
		}
	}
}

func TestListRejectsIncompleteAndMalformedPages(t *testing.T) {
	cases := []struct {
		name string
		list func(path string) (any, int, int)
	}{
		{
			name: "truncated second page",
			list: func(string) (any, int, int) {
				return map[string]any{"content": []map[string]any{}, "total": perPage + 1}, 200, 200
			},
		},
		{
			name: "total changes",
			list: func(path string) (any, int, int) {
				if path == "/p1" {
					return map[string]any{"content": []map[string]any{{"name": "A.mkv", "is_dir": false, "size": 0}}, "total": perPage + 5}, 200, 200
				}
				return map[string]any{"content": []map[string]any{{"name": "B.mkv", "is_dir": false, "size": 0}}, "total": perPage + 6}, 200, 200
			},
		},
		{
			name: "duplicate names",
			list: func(string) (any, int, int) {
				return map[string]any{"content": []map[string]any{
					{"name": "A.mkv", "is_dir": false, "size": 0},
					{"name": "A.mkv", "is_dir": false, "size": 0},
				}, "total": 2}, 200, 200
			},
		},
		{
			name: "null content with non-zero total",
			list: func(string) (any, int, int) {
				return map[string]any{"content": nil, "total": 3}, 200, 200
			},
		},
		{
			name: "null content on a later page",
			list: func(path string) (any, int, int) {
				if path == "/root" {
					content := make([]map[string]any, 0, perPage)
					for i := 0; i < perPage; i++ {
						content = append(content, map[string]any{"name": fmt.Sprintf("F%03d.mkv", i), "is_dir": false, "size": 0})
					}
					return map[string]any{"content": content, "total": perPage + 1}, 200, 200
				}
				return map[string]any{"content": nil, "total": perPage + 1}, 200, 200
			},
		},
		{
			name: "oversized directory",
			list: func(string) (any, int, int) {
				return map[string]any{"content": nil, "total": MaxDirectoryEntries + 1}, 200, 200
			},
		},
		{
			name: "application 403",
			list: func(string) (any, int, int) { return nil, 200, 403 },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := &absenceServer{list: tc.list}
			ts := srv.serve()
			defer ts.Close()
			path := "/root"
			if tc.name == "total changes" {
				path = "/p1"
			}
			if _, err := New(ts.URL, "t").List(context.Background(), path); err == nil {
				t.Fatal("incomplete listing must fail")
			}
			for _, req := range srv.listed() {
				if req.Refresh {
					t.Fatal("a failing listing must still not request a refresh")
				}
			}
		})
	}
}

func TestListRespectsCallerCancellation(t *testing.T) {
	srv := &absenceServer{list: func(string) (any, int, int) {
		time.Sleep(200 * time.Millisecond)
		return map[string]any{"content": nil, "total": 0}, 200, 200
	}}
	ts := srv.serve()
	defer ts.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New(ts.URL, "t").List(ctx, "/Movies"); err == nil {
		t.Fatal("cancellation must fail the listing")
	}
}

func TestGetRejectsMissingIdentityFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		data any
	}{
		{name: "missing name", data: map[string]any{"is_dir": false, "size": 1}},
		{name: "missing is_dir", data: map[string]any{"name": "A.mkv", "size": 1}},
		{name: "not an object", data: []any{"x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := &absenceServer{get: func(string) (any, int, int) { return tc.data, 200, 200 }}
			ts := srv.serve()
			defer ts.Close()
			if _, err := New(ts.URL, "t").Get(context.Background(), "/A.mkv"); err == nil {
				t.Fatal("an object without name and is_dir must fail")
			}
		})
	}
}
