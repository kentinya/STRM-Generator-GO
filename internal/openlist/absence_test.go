package openlist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// absenceServer is a programmable OpenList API stub. Unlike the older
// fakeServer it records every request so tests can assert freshness, request
// counts and cancellation instead of only the final reason string.
type absenceServer struct {
	mu       sync.Mutex
	requests []recordedRequest
	list     func(path string) (any, int, int)
	get      func(path string) (any, int, int)
	delay    time.Duration
}

type recordedRequest struct {
	Endpoint string
	Path     string
	Page     int
	PerPage  int
	Refresh  bool
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

// currentDelay reads the configured delay under the server mutex so a test that
// changes it while a request is in flight cannot race the handler.
func (s *absenceServer) currentDelay() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.delay
}

// setList replaces the list handler under the server mutex.
func (s *absenceServer) setList(handler func(string) (any, int, int)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.list = handler
}

// setDelay changes the artificial latency under the server mutex.
func (s *absenceServer) setDelay(delay time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delay = delay
}

func (s *absenceServer) allRequests() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedRequest(nil), s.requests...)
}

func (s *absenceServer) serve() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/fs/list", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path    string `json:"path"`
			Page    int    `json:"page"`
			PerPage int    `json:"per_page"`
			Refresh bool   `json:"refresh"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		s.record(recordedRequest{Endpoint: "/api/fs/list", Path: req.Path, Page: req.Page, PerPage: req.PerPage, Refresh: req.Refresh})
		if delay := s.currentDelay(); delay > 0 {
			time.Sleep(delay)
		}
		s.mu.Lock()
		handler := s.list
		s.mu.Unlock()
		data, status, code := handler(req.Path)
		writeEnvelope(w, status, code, data)
	})
	mux.HandleFunc("/api/fs/get", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Path string }
		json.NewDecoder(r.Body).Decode(&req)
		s.record(recordedRequest{Endpoint: "/api/fs/get", Path: req.Path})
		s.mu.Lock()
		handler := s.get
		s.mu.Unlock()
		data, status, code := handler(req.Path)
		writeEnvelope(w, status, code, data)
	})
	return httptest.NewServer(mux)
}

// node is one synthetic remote object. isDir objects are directories and are
// listed by name; files are listed too but cannot be descended into.
type node struct {
	isDir    bool
	virtual  bool
	children []string
}

// treeServer builds an absenceServer over a path -> node map. provider is
// emitted on Get so the client can distinguish real storage objects from
// virtual mount ancestors, mirroring the pinned OpenList build.
func treeServer(tree map[string]node) *absenceServer {
	return &absenceServer{
		list: func(path string) (any, int, int) {
			n, ok := tree[path]
			if !ok || !n.isDir {
				return nil, 200, 500
			}
			names := append([]string(nil), n.children...)
			if len(names) == 0 {
				return map[string]any{"content": nil, "total": 0}, 200, 200
			}
			content := make([]map[string]any, 0, len(names))
			for _, name := range names {
				child := tree[joinRemote(path, name)]
				content = append(content, map[string]any{
					"name": name, "is_dir": child.isDir, "size": 0,
				})
			}
			return map[string]any{"content": content, "total": len(content)}, 200, 200
		},
		get: func(path string) (any, int, int) {
			n, ok := tree[path]
			if !ok {
				return nil, 200, 500
			}
			provider := "Local"
			if n.virtual {
				provider = "unknown"
			}
			name := path
			if idx := strings.LastIndex(path, "/"); idx >= 0 {
				name = path[idx+1:]
			}
			if path == "/" {
				name = "root"
			}
			return map[string]any{
				"name": name, "is_dir": n.isDir, "size": 0, "provider": provider,
			}, 200, 200
		},
	}
}

func newTreeServer(t *testing.T, tree map[string]node) (*Confirmer, *absenceServer) {
	t.Helper()
	srv := treeServer(tree)
	ts := srv.serve()
	t.Cleanup(ts.Close)
	return NewConfirmer(New(ts.URL, "token")), srv
}

func dirNode(children ...string) node { return node{isDir: true, children: children} }
func fileNode() node                  { return node{} }

func TestConfirmExactPresentAndAbsent(t *testing.T) {
	tree := map[string]node{
		"/Root":                  dirNode("Movies", "Notes.txt"),
		"/Root/Movies":           dirNode("A.mkv", "Sub"),
		"/Root/Movies/A.mkv":     fileNode(),
		"/Root/Movies/Sub":       dirNode("B.mkv"),
		"/Root/Movies/Sub/B.mkv": fileNode(),
		"/Root/Notes.txt":        fileNode(),
	}
	c, _ := newTreeServer(t, tree)
	ctx := context.Background()

	got := c.Confirm(ctx, "/Root", "/Root/Movies/A.mkv")
	if got.Status != AbsencePresent {
		t.Fatalf("existing deep file: %+v", got)
	}
	got = c.Confirm(ctx, "/Root", "/Root/Movies/Missing.mkv")
	if got.Status != AbsenceConfirmed || got.ConfirmedParent != "/Root/Movies" || got.ConfirmedName != "Missing.mkv" {
		t.Fatalf("missing file: %+v", got)
	}
	// The legacy file is not media but confirmation is about a name, not media
	// rules: a non-media name must still be reported present.
	got = c.Confirm(ctx, "/Root", "/Root/Notes.txt")
	if got.Status != AbsencePresent {
		t.Fatalf("non-media name must be present: %+v", got)
	}
}

func TestConfirmCaseAndSimilarPrefixAreNotConflated(t *testing.T) {
	tree := map[string]node{
		"/Root":              dirNode("Movies", "Movies2", "movies"),
		"/Root/Movies":       dirNode("A.mkv"),
		"/Root/Movies/A.mkv": fileNode(),
		"/Root/Movies2":      dirNode(),
		"/Root/movies":       dirNode(),
	}
	c, _ := newTreeServer(t, tree)
	ctx := context.Background()
	for _, target := range []string{"/Root/Movies/a.mkv", "/Root/Movies/A.MKV"} {
		got := c.Confirm(ctx, "/Root", target)
		if got.Status != AbsenceConfirmed {
			t.Fatalf("%s must be confirmed absent, got %+v", target, got)
		}
	}
	// A prefix that is not a full segment must be rejected before any request.
	c2, srv := newTreeServer(t, tree)
	if got := c2.Confirm(ctx, "/Root", "/Root/Movies2"); got.Status != AbsencePresent {
		t.Fatalf("Movies2 must be present: %+v", got)
	}
	if got := c2.Confirm(ctx, "/Root", "/Root/Movie"); got.Status != AbsenceConfirmed {
		t.Fatalf("Movie is a genuinely absent name: %+v", got)
	}
	if n := len(srv.allRequests()); n == 0 {
		t.Fatal("confirmation made no requests")
	}
}

func TestConfirmAncestorRemovedWithinSource(t *testing.T) {
	tree := map[string]node{
		"/Root":        dirNode("Movies", "TV"),
		"/Root/Movies": dirNode(),
		"/Root/TV":     dirNode(),
	}
	c, srv := newTreeServer(t, tree)
	ctx := context.Background()
	got := c.Confirm(ctx, "/Root", "/Root/Gone/Season 1/Episode.mkv")
	if got.Status != AbsenceConfirmed {
		t.Fatalf("removed ancestor must be confirmed absent: %+v", got)
	}
	if got.ConfirmedParent != "/Root" || got.ConfirmedName != "Gone" {
		t.Fatalf("the chain proof must name the first missing segment: %+v", got)
	}
	// No request may be issued for the missing subtree.
	for _, req := range srv.allRequests() {
		if strings.HasPrefix(req.Path, "/Root/Gone") {
			t.Fatalf("issued a request for a missing subtree: %+v", req)
		}
	}
}

func TestConfirmRequiresFreshListingOnEveryPage(t *testing.T) {
	first := make([]map[string]any, 0, perPage)
	for i := 0; i < perPage; i++ {
		first = append(first, map[string]any{"name": fmt.Sprintf("F%04d.mkv", i), "is_dir": false, "size": 0})
	}
	second := []map[string]any{{"name": "Target.mkv", "is_dir": false, "size": 0}}
	total := perPage + 1
	var srv *absenceServer
	srv = &absenceServer{
		list: func(path string) (any, int, int) {
			srv.mu.Lock()
			page := 1
			for _, r := range srv.requests {
				if r.Endpoint == "/api/fs/list" {
					page = r.Page
				}
			}
			srv.mu.Unlock()
			if page == 1 {
				return map[string]any{"content": first, "total": total}, 200, 200
			}
			return map[string]any{"content": second, "total": total}, 200, 200
		},
		get: func(string) (any, int, int) {
			return map[string]any{"name": "Root", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
		},
	}
	ts := srv.serve()
	defer ts.Close()
	c := NewConfirmer(New(ts.URL, "t"))
	got := c.Confirm(context.Background(), "/Root", "/Root/Target.mkv")
	if got.Status != AbsencePresent {
		t.Fatalf("target on the last page must be present: %+v", got)
	}
	pages := map[int]bool{}
	for _, req := range srv.listed() {
		if !req.Refresh {
			t.Fatalf("page %d was not refreshed: %+v", req.Page, req)
		}
		pages[req.Page] = true
	}
	if !pages[1] || !pages[2] {
		t.Fatalf("expected both pages to be read, got %v", pages)
	}
}

func TestConfirmRejectsUnlistedAncestorWithVirtualChild(t *testing.T) {
	// A directory that is only a virtual ancestor of a nested mount reports
	// provider "unknown" and can be listed while its own driver is unreachable.
	tree := map[string]node{
		"/Root":        dirNode("Movies"),
		"/Root/Movies": {isDir: true, virtual: true, children: []string{"NestedMount"}},
	}
	c, _ := newTreeServer(t, tree)
	got := c.Confirm(context.Background(), "/Root", "/Root/Movies/Gone.mkv")
	if got.Status != AbsenceUnknown {
		t.Fatalf("virtual ancestor must not confirm absence: %+v", got)
	}
	if got.Reason != ReasonVirtualViewObserved {
		t.Fatalf("expected the virtual view reason, got %+v", got)
	}
}

func TestConfirmRejectsVirtualRoot(t *testing.T) {
	tree := map[string]node{
		"/Root": {isDir: true, virtual: true, children: []string{"Movies"}},
	}
	c, _ := newTreeServer(t, tree)
	if got := c.Confirm(context.Background(), "/Root", "/Root/Movies/A.mkv"); got.Status != AbsenceUnknown {
		t.Fatalf("virtual root must not confirm absence: %+v", got)
	}
}

func TestConfirmFailuresAreNotAbsence(t *testing.T) {
	cases := []struct {
		name string
		list func(path string) (any, int, int)
		get  func(path string) (any, int, int)
	}{
		{
			name: "root get fails",
			list: func(string) (any, int, int) { return map[string]any{"content": nil, "total": 0}, 200, 200 },
			get:  func(string) (any, int, int) { return nil, 200, 500 },
		},
		{
			name: "root get forbidden",
			list: func(string) (any, int, int) { return map[string]any{"content": nil, "total": 0}, 200, 200 },
			get:  func(string) (any, int, int) { return nil, 200, 403 },
		},
		{
			name: "root http 503",
			list: func(string) (any, int, int) { return map[string]any{"content": nil, "total": 0}, 200, 200 },
			get:  func(string) (any, int, int) { return nil, 503, 503 },
		},
		{
			name: "parent list 403",
			list: func(path string) (any, int, int) {
				if path == "/Root" {
					return map[string]any{"content": []map[string]any{{"name": "Sub", "is_dir": true, "size": 0}}, "total": 1}, 200, 200
				}
				return nil, 200, 403
			},
			get: func(string) (any, int, int) {
				return map[string]any{"name": "d", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
			},
		},
		{
			name: "parent list application 500",
			list: func(path string) (any, int, int) {
				if path == "/Root" {
					return map[string]any{"content": []map[string]any{{"name": "Sub", "is_dir": true, "size": 0}}, "total": 1}, 200, 200
				}
				return nil, 200, 500
			},
			get: func(string) (any, int, int) {
				return map[string]any{"name": "d", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
			},
		},
		{
			name: "refresh refused",
			list: func(string) (any, int, int) { return nil, 200, 403 },
			get: func(string) (any, int, int) {
				return map[string]any{"name": "d", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
			},
		},
		{
			name: "truncated second page",
			list: func(path string) (any, int, int) {
				return map[string]any{"content": []map[string]any{}, "total": perPage + 1}, 200, 200
			},
			get: func(string) (any, int, int) {
				return map[string]any{"name": "d", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
			},
		},
		{
			name: "total changes",
			list: func(path string) (any, int, int) {
				return map[string]any{"content": []map[string]any{{"name": "A.mkv", "is_dir": false, "size": 0}}, "total": perPage + 5}, 200, 200
			},
			get: func(string) (any, int, int) {
				return map[string]any{"name": "d", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
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
			get: func(string) (any, int, int) {
				return map[string]any{"name": "d", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
			},
		},
		{
			name: "null content with non-zero total",
			list: func(string) (any, int, int) {
				return map[string]any{"content": nil, "total": 3}, 200, 200
			},
			get: func(string) (any, int, int) {
				return map[string]any{"name": "d", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
			},
		},
		{
			name: "root is a file",
			list: func(string) (any, int, int) { return map[string]any{"content": nil, "total": 0}, 200, 200 },
			get: func(string) (any, int, int) {
				return map[string]any{"name": "Root", "is_dir": false, "size": 0, "provider": "Local"}, 200, 200
			},
		},
		{
			name: "oversized directory total",
			list: func(string) (any, int, int) {
				return map[string]any{"content": nil, "total": MaxDirectoryEntries + 1}, 200, 200
			},
			get: func(string) (any, int, int) {
				return map[string]any{"name": "d", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := &absenceServer{list: tc.list, get: tc.get}
			ts := srv.serve()
			defer ts.Close()
			c := NewConfirmer(New(ts.URL, "t"))
			got := c.Confirm(context.Background(), "/Root", "/Root/Sub/A.mkv")
			if got.Status == AbsenceConfirmed {
				t.Fatalf("%s must not confirm absence: %+v", tc.name, got)
			}
		})
	}
}

func TestConfirmIntermediateIsNotADirectory(t *testing.T) {
	tree := map[string]node{
		"/Root":              dirNode("Movies"),
		"/Root/Movies":       dirNode("A.mkv"),
		"/Root/Movies/A.mkv": fileNode(),
	}
	c, _ := newTreeServer(t, tree)
	got := c.Confirm(context.Background(), "/Root", "/Root/Movies/A.mkv/Child")
	if got.Status != AbsenceUnknown {
		t.Fatalf("a file component must not yield absence: %+v", got)
	}
}

func TestConfirmTargetDirectoryIsPresentNotAbsent(t *testing.T) {
	tree := map[string]node{
		"/Root":     dirNode("Sub"),
		"/Root/Sub": dirNode(),
	}
	c, _ := newTreeServer(t, tree)
	if got := c.Confirm(context.Background(), "/Root", "/Root/Sub"); got.Status != AbsencePresent {
		t.Fatalf("a directory target must be present: %+v", got)
	}
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
		{"", "/Root/A.mkv", ReasonInvalidInput},
		{"/Root", "", ReasonInvalidInput},
		{"/", "/", ReasonRootIsTarget},
	}
	for _, tc := range cases {
		srv := &absenceServer{
			list: func(string) (any, int, int) { return map[string]any{"content": nil, "total": 0}, 200, 200 },
			get:  func(string) (any, int, int) { return nil, 200, 500 },
		}
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

func TestConfirmSourceRootSlash(t *testing.T) {
	tree := map[string]node{
		"/":             {isDir: true, children: []string{"Movies"}},
		"/Movies":       dirNode("A.mkv"),
		"/Movies/A.mkv": fileNode(),
	}
	c, _ := newTreeServer(t, tree)
	ctx := context.Background()
	if got := c.Confirm(ctx, "/", "/Movies/A.mkv"); got.Status != AbsencePresent {
		t.Fatalf("root-relative present: %+v", got)
	}
	if got := c.Confirm(ctx, "/", "/Movies/B.mkv"); got.Status != AbsenceConfirmed {
		t.Fatalf("root-relative absent: %+v", got)
	}
	if got := c.Confirm(ctx, "/", "/Movies"); got.Status != AbsencePresent {
		t.Fatalf("root-relative directory: %+v", got)
	}
}

func TestConfirmDepthLimit(t *testing.T) {
	segments := make([]string, MaxAbsenceDepth+1)
	for i := range segments {
		segments[i] = fmt.Sprintf("d%d", i)
	}
	target := "/Root/" + strings.Join(segments, "/")
	srv := &absenceServer{
		list: func(string) (any, int, int) { return map[string]any{"content": nil, "total": 0}, 200, 200 },
		get:  func(string) (any, int, int) { return nil, 200, 500 },
	}
	ts := srv.serve()
	defer ts.Close()
	got := NewConfirmer(New(ts.URL, "t")).Confirm(context.Background(), "/Root", target)
	if got.Reason != ReasonDepthLimit {
		t.Fatalf("depth limit: %+v", got)
	}
	if n := len(srv.allRequests()); n != 0 {
		t.Fatalf("depth limit must be rejected before any request, got %d", n)
	}
}

func TestConfirmRequestBudgetIsBounded(t *testing.T) {
	// Build a deep chain where every level exists as a real directory, so the
	// confirmation walks the full depth and then reads the final parent.
	const depth = 12
	tree := map[string]node{}
	path := "/Root"
	tree[path] = node{isDir: true, children: []string{"d0"}}
	for i := 1; i <= depth; i++ {
		next := joinRemote(path, fmt.Sprintf("d%d", i-1))
		tree[path] = node{isDir: true, children: []string{fmt.Sprintf("d%d", i-1)}}
		path = next
	}
	tree[path] = dirNode("A.mkv")
	c, srv := newTreeServer(t, tree)
	got := c.Confirm(context.Background(), "/Root", path+"/A.mkv")
	if got.Status != AbsencePresent {
		t.Fatalf("deep present: %+v", got)
	}
	if n := len(srv.allRequests()); n > MaxAbsenceRequests {
		t.Fatalf("request budget exceeded: %d > %d", n, MaxAbsenceRequests)
	}
}

func TestConfirmCancellationAndTimeout(t *testing.T) {
	tree := map[string]node{"/Root": dirNode("A.mkv")}
	srv := treeServer(tree)
	srv.setDelay(200 * time.Millisecond)
	ts := srv.serve()
	defer ts.Close()
	c := NewConfirmer(New(ts.URL, "t").WithHTTPClient(&http.Client{Timeout: 20 * time.Millisecond}))
	if got := c.Confirm(context.Background(), "/Root", "/Root/Missing.mkv"); got.Status != AbsenceUnknown {
		t.Fatalf("timeout must not confirm absence: %+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	srv.setDelay(0)
	if got := c.Confirm(ctx, "/Root", "/Root/Missing.mkv"); got.Status != AbsenceUnknown || got.Reason != ReasonCancelled {
		t.Fatalf("cancelled context must not confirm absence: %+v", got)
	}
}

func TestConfirmDoesNotReusePreviousResult(t *testing.T) {
	// The confirming client must re-read facts every call: a name that appears
	// after an absent result becomes present on the next call.
	present := false
	srv := &absenceServer{
		list: func(string) (any, int, int) {
			if present {
				return map[string]any{"content": []map[string]any{{"name": "A.mkv", "is_dir": false, "size": 0}}, "total": 1}, 200, 200
			}
			return map[string]any{"content": nil, "total": 0}, 200, 200
		},
		get: func(path string) (any, int, int) {
			if strings.HasSuffix(path, "/A.mkv") && !present {
				// The pinned build answers a missing object with application 500.
				return nil, 200, 500
			}
			return map[string]any{"name": "d", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
		},
	}
	ts := srv.serve()
	defer ts.Close()
	c := NewConfirmer(New(ts.URL, "t"))
	ctx := context.Background()
	if got := c.Confirm(ctx, "/Root", "/Root/A.mkv"); got.Status != AbsenceConfirmed {
		t.Fatalf("initial absence: %+v", got)
	}
	present = true
	if got := c.Confirm(ctx, "/Root", "/Root/A.mkv"); got.Status != AbsencePresent {
		t.Fatalf("reappeared name must be present: %+v", got)
	}
	if !srv.listed()[0].Refresh || !srv.listed()[len(srv.listed())-1].Refresh {
		t.Fatal("every confirmation listing must force a refresh")
	}
}

func TestConfirmConcurrentCallsKeepIndependentState(t *testing.T) {
	var gets int64
	srv := &absenceServer{
		list: func(path string) (any, int, int) {
			return map[string]any{"content": nil, "total": 0}, 200, 200
		},
		get: func(path string) (any, int, int) {
			atomic.AddInt64(&gets, 1)
			if strings.HasPrefix(path, "/Root/A") {
				// Absent target: the fixed build reports application code 500.
				return nil, 200, 500
			}
			return map[string]any{"name": "d", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
		},
	}
	ts := srv.serve()
	defer ts.Close()
	c := NewConfirmer(New(ts.URL, "t"))
	var wg sync.WaitGroup
	results := make([]AbsenceResult, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = c.Confirm(context.Background(), "/Root", fmt.Sprintf("/Root/A%d.mkv", i))
		}(i)
	}
	wg.Wait()
	for i, got := range results {
		if got.Status != AbsenceConfirmed || got.ConfirmedName != fmt.Sprintf("A%d.mkv", i) {
			t.Fatalf("result %d leaked state: %+v", i, got)
		}
	}
	if n := atomic.LoadInt64(&gets); n != 2*int64(len(results)) {
		t.Fatalf("expected one root read and one target probe per call, got %d for %d calls",
			n, len(results))
	}
}

func TestConfirmZeroValueIsNotAbsence(t *testing.T) {
	var zero AbsenceResult
	if zero.Status == AbsenceConfirmed || zero.Status.String() != "unknown" {
		t.Fatalf("zero result must mean unknown: %+v", zero)
	}
	// A nil confirmer must never answer confirmed-absent.
	var nilConfirmer *Confirmer
	if got := nilConfirmer.Confirm(context.Background(), "/Root", "/Root/A.mkv"); got.Status != AbsenceUnknown {
		t.Fatalf("nil confirmer: %+v", got)
	}
	if got := NewConfirmer(nil).Confirm(context.Background(), "/Root", "/Root/A.mkv"); got.Status != AbsenceUnknown {
		t.Fatalf("nil lister: %+v", got)
	}
}

func TestConfirmReasonHasNoSecrets(t *testing.T) {
	srv := &absenceServer{
		list: func(string) (any, int, int) { return nil, 200, 500 },
		get:  func(string) (any, int, int) { return nil, 200, 500 },
	}
	ts := srv.serve()
	defer ts.Close()
	got := NewConfirmer(New(ts.URL, "super-secret-token")).Confirm(context.Background(), "/Root", "/Root/A.mkv")
	text := string(got.Reason)
	for _, banned := range []string{"super-secret-token", "http", "APIError", "failed to get"} {
		if strings.Contains(text, banned) {
			t.Fatalf("reason leaked %q: %q", banned, text)
		}
	}
	var apiErr *APIError
	if errors.As(errors.New("x"), &apiErr) {
		t.Fatal("sanity")
	}
}

func TestListFreshRequestsRefreshOnEveryPage(t *testing.T) {
	all := make([]map[string]any, 0, perPage+1)
	for i := 0; i < perPage+1; i++ {
		all = append(all, map[string]any{"name": fmt.Sprintf("F%03d.mkv", i), "is_dir": false, "size": 0})
	}
	srv := &absenceServer{
		list: func(path string) (any, int, int) { return nil, 200, 200 },
		get:  func(string) (any, int, int) { return nil, 200, 500 },
	}
	srv.setList(func(path string) (any, int, int) {
		srv.mu.Lock()
		page := 1
		for _, r := range srv.requests {
			if r.Endpoint == "/api/fs/list" {
				page = r.Page
			}
		}
		srv.mu.Unlock()
		start := (page - 1) * perPage
		end := start + perPage
		if start > len(all) {
			start = len(all)
		}
		if end > len(all) {
			end = len(all)
		}
		return map[string]any{"content": all[start:end], "total": len(all)}, 200, 200
	})
	ts := srv.serve()
	defer ts.Close()
	entries, err := New(ts.URL, "t").ListFresh(context.Background(), "/Movies")
	if err != nil {
		t.Fatalf("list fresh: %v", err)
	}
	if len(entries) != perPage+1 {
		t.Fatalf("got %d entries", len(entries))
	}
	pages := map[int]bool{}
	for _, req := range srv.listed() {
		if !req.Refresh {
			t.Fatalf("page %d not refreshed", req.Page)
		}
		pages[req.Page] = true
	}
	if !pages[1] || !pages[2] {
		t.Fatalf("expected 2 pages, got %v", pages)
	}
}

func TestListFreshDoesNotFallBackToCachedList(t *testing.T) {
	var refreshCalls int64
	srv := &absenceServer{
		list: func(string) (any, int, int) { return nil, 200, 403 },
		get:  func(string) (any, int, int) { return nil, 200, 500 },
	}
	srv.setList(func(path string) (any, int, int) {
		atomic.AddInt64(&refreshCalls, 1)
		return map[string]any{"content": nil, "total": 0}, 200, 403
	})
	ts := srv.serve()
	defer ts.Close()
	if _, err := New(ts.URL, "t").ListFresh(context.Background(), "/Movies"); err == nil {
		t.Fatal("refresh refusal must be an error")
	}
	if atomic.LoadInt64(&refreshCalls) != 1 {
		t.Fatal("a refused refresh must not be retried as a cached list")
	}
	for _, req := range srv.listed() {
		if !req.Refresh {
			t.Fatal("ListFresh issued a non-refreshing request")
		}
	}
}

func TestListFreshAcceptsRealEmptyDirectoryShape(t *testing.T) {
	srv := &absenceServer{
		list: func(string) (any, int, int) {
			return map[string]any{"content": nil, "total": 0}, 200, 200
		},
		get: func(string) (any, int, int) { return nil, 200, 500 },
	}
	ts := srv.serve()
	defer ts.Close()
	entries, err := New(ts.URL, "t").ListFresh(context.Background(), "/Movies")
	if err != nil || len(entries) != 0 {
		t.Fatalf("real empty directory: entries=%v err=%v", entries, err)
	}
}

func TestGetMarksVirtualDirectory(t *testing.T) {
	srv := &absenceServer{
		list: func(string) (any, int, int) { return map[string]any{"content": nil, "total": 0}, 200, 200 },
		get: func(path string) (any, int, int) {
			provider := "Local"
			if path == "/virtual" {
				provider = "unknown"
			}
			return map[string]any{"name": "n", "is_dir": true, "size": 0, "provider": provider}, 200, 200
		},
	}
	ts := srv.serve()
	defer ts.Close()
	c := New(ts.URL, "t")
	real, err := c.Get(context.Background(), "/real")
	if err != nil || real.Virtual {
		t.Fatalf("real directory must not be virtual: %+v %v", real, err)
	}
	virt, err := c.Get(context.Background(), "/virtual")
	if err != nil || !virt.Virtual {
		t.Fatalf("virtual directory must be marked: %+v %v", virt, err)
	}
	// A file with an unknown provider is not a virtual directory view. The new
	// handler is installed on a separate server so the running one is untouched.
	fileSrv := &absenceServer{
		list: func(string) (any, int, int) { return map[string]any{"content": nil, "total": 0}, 200, 200 },
		get: func(string) (any, int, int) {
			return map[string]any{"name": "f", "is_dir": false, "size": 1, "provider": "unknown"}, 200, 200
		},
	}
	fileTS := fileSrv.serve()
	defer fileTS.Close()
	file, err := New(fileTS.URL, "t").Get(context.Background(), "/file")
	if err != nil || file.Virtual {
		t.Fatalf("a file is never a virtual directory: %+v %v", file, err)
	}
}

func TestConfirmFilteredTargetIsNotAbsent(t *testing.T) {
	// OpenList removes entries the requesting identity may not see from a
	// listing, so a filtered name looks exactly like a deleted one. A structural
	// permission refusal on the direct read must keep the answer unknown.
	srv := &absenceServer{
		list: func(string) (any, int, int) {
			return map[string]any{"content": []map[string]any{{"name": "Other.mkv", "is_dir": false, "size": 0}}, "total": 1}, 200, 200
		},
		get: func(path string) (any, int, int) {
			if strings.HasSuffix(path, "/Hidden.mkv") {
				return nil, 200, 403
			}
			return map[string]any{"name": "d", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
		},
	}
	ts := srv.serve()
	defer ts.Close()
	got := NewConfirmer(New(ts.URL, "t")).Confirm(context.Background(), "/Root", "/Root/Hidden.mkv")
	if got.Status != AbsenceUnknown || got.Reason != ReasonPermissionFiltered {
		t.Fatalf("a filtered name must not be confirmed absent: %+v", got)
	}
}

func TestConfirmTargetAppearingInDirectReadIsUnknown(t *testing.T) {
	// If the listing omits the name but the direct read finds it, the two facts
	// disagree and the result must not be absence.
	srv := &absenceServer{
		list: func(string) (any, int, int) {
			return map[string]any{"content": nil, "total": 0}, 200, 200
		},
		get: func(path string) (any, int, int) {
			if strings.HasSuffix(path, "/Ghost.mkv") {
				return map[string]any{"name": "Ghost.mkv", "is_dir": false, "size": 1, "provider": "Local"}, 200, 200
			}
			return map[string]any{"name": "d", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
		},
	}
	ts := srv.serve()
	defer ts.Close()
	got := NewConfirmer(New(ts.URL, "t")).Confirm(context.Background(), "/Root", "/Root/Ghost.mkv")
	if got.Status == AbsenceConfirmed {
		t.Fatalf("a contradicting direct read must not confirm absence: %+v", got)
	}
}

func TestConfirmGenericGetFailureIsAbsenceOnlyAfterListing(t *testing.T) {
	// The pinned build answers a missing object and a driver failure with the
	// same application code, so the listing is what establishes absence; the
	// direct read only has to avoid contradicting it.
	srv := &absenceServer{
		list: func(string) (any, int, int) {
			return map[string]any{"content": nil, "total": 0}, 200, 200
		},
		get: func(path string) (any, int, int) {
			if path == "/Root" {
				return map[string]any{"name": "Root", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
			}
			return nil, 200, 500
		},
	}
	ts := srv.serve()
	defer ts.Close()
	got := NewConfirmer(New(ts.URL, "t")).Confirm(context.Background(), "/Root", "/Root/Gone.mkv")
	if got.Status != AbsenceConfirmed {
		t.Fatalf("a genuinely missing name must be confirmed: %+v", got)
	}
}

func TestIsPermissionDeniedUsesStructuralCode(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{&APIError{Op: "x", Detail: "application code 403", Code: 403}, true},
		{&APIError{Op: "x", Detail: "http status 403", HTTPStatus: 403}, true},
		{&APIError{Op: "x", Detail: "application code 500", Code: 500}, false},
		// Message text mentioning 403 must not be treated as a permission code.
		{&APIError{Op: "x", Detail: "upstream said 403 but code was 500", Code: 500}, false},
		{&APIError{Op: "x", Detail: "transport failure"}, false},
		{context.Canceled, false},
		{fmt.Errorf("wrapped: %w", &APIError{Detail: "application code 403", Code: 403}), true},
	}
	for _, tc := range cases {
		if got := IsPermissionDenied(tc.err); got != tc.want {
			t.Fatalf("IsPermissionDenied(%v)=%v want %v", tc.err, got, tc.want)
		}
	}
}

func TestConcurrentConfirmationsOnSharedClientDoNotSharePagination(t *testing.T) {
	// One Client is shared by every concurrent confirmation. The pagination
	// cursor must live on the call stack, so two different directories listed at
	// the same time can never consume each other's pages.
	const workers = 6
	pageCount := 3
	makePage := func(prefix string, page int) []map[string]any {
		entries := make([]map[string]any, 0, perPage)
		for i := 0; i < perPage; i++ {
			entries = append(entries, map[string]any{
				"name": fmt.Sprintf("%s-%d-%04d.mkv", prefix, page, i), "is_dir": false, "size": 0,
			})
		}
		return entries
	}
	total := pageCount * perPage
	var srv *absenceServer
	srv = &absenceServer{
		list: func(path string) (any, int, int) {
			if path == "/Root" {
				// /Root only enumerates the per-worker directories; each of those
				// is read as a multi-page directory below.
				content := make([]map[string]any, 0, workers)
				for i := 0; i < workers; i++ {
					content = append(content, map[string]any{
						"name": fmt.Sprintf("Dir%d", i), "is_dir": true, "size": 0,
					})
				}
				return map[string]any{"content": content, "total": len(content)}, 200, 200
			}
			srv.mu.Lock()
			page := 1
			for _, r := range srv.requests {
				if r.Endpoint == "/api/fs/list" && r.Path == path {
					page = r.Page
				}
			}
			srv.mu.Unlock()
			prefix := strings.ReplaceAll(strings.TrimPrefix(path, "/Root/"), "/", "_")
			return map[string]any{"content": makePage(prefix, page), "total": total}, 200, 200
		},
		get: func(path string) (any, int, int) {
			if path == "/Root" || strings.HasPrefix(path, "/Root/Dir") && !strings.HasSuffix(path, "Absent.mkv") {
				return map[string]any{"name": "d", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
			}
			return nil, 200, 500
		},
	}
	ts := srv.serve()
	defer ts.Close()
	client := New(ts.URL, "t")
	confirmer := NewConfirmer(client)
	var wg sync.WaitGroup
	errs := make([]AbsenceResult, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = confirmer.Confirm(context.Background(),
				"/Root", fmt.Sprintf("/Root/Dir%d/Absent.mkv", i))
		}(i)
	}
	wg.Wait()
	for i, got := range errs {
		if got.Status != AbsenceConfirmed {
			t.Fatalf("worker %d: %+v", i, got)
		}
		if got.ConfirmedParent != fmt.Sprintf("/Root/Dir%d", i) {
			t.Fatalf("worker %d got another directory's result: %+v", i, got)
		}
	}
	// Each worker must have read all three pages of its own directory exactly
	// once: the target sits directly inside that directory, so its fresh listing
	// is the evidence that proves absence.
	perDir := map[string]int{}
	for _, req := range srv.listed() {
		perDir[req.Path]++
	}
	for i := 0; i < workers; i++ {
		want := fmt.Sprintf("/Root/Dir%d", i)
		if perDir[want] != pageCount {
			t.Fatalf("%s read %d pages, want %d", want, perDir[want], pageCount)
		}
	}
}

func TestConfirmBoundsRetainedEntriesPerDirectory(t *testing.T) {
	// A directory whose declared total exceeds the documented limit must be
	// rejected instead of accumulated, so one confirmation cannot grow memory
	// beyond the bounded directory size.
	srv := &absenceServer{
		list: func(string) (any, int, int) {
			return map[string]any{"content": nil, "total": MaxDirectoryEntries + 1}, 200, 200
		},
		get: func(string) (any, int, int) {
			return map[string]any{"name": "d", "is_dir": true, "size": 0, "provider": "Local"}, 200, 200
		},
	}
	ts := srv.serve()
	defer ts.Close()
	got := NewConfirmer(New(ts.URL, "t")).Confirm(context.Background(), "/Root", "/Root/A.mkv")
	if got.Status != AbsenceUnknown {
		t.Fatalf("an over-limit directory must not confirm absence: %+v", got)
	}
}
