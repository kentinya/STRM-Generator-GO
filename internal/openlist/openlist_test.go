package openlist

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeServer is a programmable OpenList API stub for client tests. It is
// intentionally separate from the Python fixture; it exercises transport and
// shape edge cases the fixture does not cover.
type fakeServer struct {
	listHandler func(page, perPage int) (any, int, int) // data, httpStatus, code
	getHandler  func(path string) (any, int, int)
}

func (f *fakeServer) serve() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/fs/list", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path    string `json:"path"`
			Page    int    `json:"page"`
			PerPage int    `json:"per_page"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		data, status, code := f.listHandler(req.Page, req.PerPage)
		writeEnvelope(w, status, code, data)
	})
	mux.HandleFunc("/api/fs/get", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Path string }
		json.NewDecoder(r.Body).Decode(&req)
		data, status, code := f.getHandler(req.Path)
		writeEnvelope(w, status, code, data)
	})
	return httptest.NewServer(mux)
}

func writeEnvelope(w http.ResponseWriter, status, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"code": code, "message": "msg", "data": data})
}

func listPage(content []map[string]any, total int) map[string]any {
	return map[string]any{"content": content, "total": total, "provider": "test"}
}

func TestListPaginates(t *testing.T) {
	all := []map[string]any{}
	for i := 0; i < 5; i++ {
		all = append(all, map[string]any{"name": fmt.Sprintf("F%d.mkv", i), "is_dir": false, "size": i})
	}
	srv := &fakeServer{listHandler: func(page, perPage int) (any, int, int) {
		start := (page - 1) * perPage
		end := start + perPage
		if start > len(all) {
			start = len(all)
		}
		if end > len(all) {
			end = len(all)
		}
		return listPage(all[start:end], len(all)), 200, 200
	}}
	ts := srv.serve()
	defer ts.Close()
	c := New(ts.URL, "token")
	entries, err := c.List(context.Background(), "/Movies")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("got %d entries", len(entries))
	}
}

func TestHTTP200Code500Fails(t *testing.T) {
	srv := &fakeServer{listHandler: func(page, perPage int) (any, int, int) { return nil, 200, 500 }}
	ts := srv.serve()
	defer ts.Close()
	if _, err := New(ts.URL, "t").List(context.Background(), "/Movies"); err == nil {
		t.Fatal("application code 500 must fail")
	}
}

func TestHTTP503Fails(t *testing.T) {
	srv := &fakeServer{listHandler: func(page, perPage int) (any, int, int) { return nil, 503, 503 }}
	ts := srv.serve()
	defer ts.Close()
	if _, err := New(ts.URL, "t").List(context.Background(), "/Movies"); err == nil {
		t.Fatal("HTTP 503 must fail")
	}
}

func TestNullDataFails(t *testing.T) {
	srv := &fakeServer{listHandler: func(page, perPage int) (any, int, int) { return nil, 200, 200 }}
	ts := srv.serve()
	defer ts.Close()
	if _, err := New(ts.URL, "t").List(context.Background(), "/Movies"); err == nil {
		t.Fatal("null data must not be treated as empty")
	}
}

func TestEarlyEmptyPageFails(t *testing.T) {
	srv := &fakeServer{listHandler: func(page, perPage int) (any, int, int) {
		if page == 1 {
			return listPage([]map[string]any{{"name": "A.mkv", "is_dir": false, "size": 1}}, 3), 200, 200
		}
		return listPage([]map[string]any{}, 3), 200, 200
	}}
	ts := srv.serve()
	defer ts.Close()
	if _, err := New(ts.URL, "t").List(context.Background(), "/Movies"); err == nil {
		t.Fatal("early empty page must fail")
	}
}

func TestDuplicateEntryFails(t *testing.T) {
	srv := &fakeServer{listHandler: func(page, perPage int) (any, int, int) {
		return listPage([]map[string]any{
			{"name": "A.mkv", "is_dir": false, "size": 1},
			{"name": "A.mkv", "is_dir": false, "size": 1},
		}, 2), 200, 200
	}}
	ts := srv.serve()
	defer ts.Close()
	if _, err := New(ts.URL, "t").List(context.Background(), "/Movies"); err == nil {
		t.Fatal("duplicate entry must fail")
	}
}

func TestTotalChangeFails(t *testing.T) {
	srv := &fakeServer{listHandler: func(page, perPage int) (any, int, int) {
		if page == 1 {
			return listPage([]map[string]any{{"name": "A.mkv", "is_dir": false, "size": 1}}, 3), 200, 200
		}
		return listPage([]map[string]any{{"name": "B.mkv", "is_dir": false, "size": 1}}, 4), 200, 200
	}}
	ts := srv.serve()
	defer ts.Close()
	if _, err := New(ts.URL, "t").List(context.Background(), "/Movies"); err == nil {
		t.Fatal("changing total must fail")
	}
}

func TestMissingTotalFails(t *testing.T) {
	srv := &fakeServer{listHandler: func(page, perPage int) (any, int, int) {
		return map[string]any{"content": []map[string]any{}}, 200, 200
	}}
	ts := srv.serve()
	defer ts.Close()
	if _, err := New(ts.URL, "t").List(context.Background(), "/Movies"); err == nil {
		t.Fatal("missing total must fail")
	}
}

func TestResponseByteLimit(t *testing.T) {
	srv := &fakeServer{listHandler: func(page, perPage int) (any, int, int) {
		big := strings.Repeat("x", int(MaxResponseBytes)+100)
		return map[string]any{"content": []map[string]any{{"name": big, "is_dir": false, "size": 1}}, "total": 1}, 200, 200
	}}
	ts := srv.serve()
	defer ts.Close()
	if _, err := New(ts.URL, "t").List(context.Background(), "/Movies"); err == nil {
		t.Fatal("oversized response must fail")
	}
}

func TestTimeout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/fs/list", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		writeEnvelope(w, 200, 200, listPage(nil, 0))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	c := New(ts.URL, "t").WithHTTPClient(&http.Client{Timeout: 20 * time.Millisecond})
	if _, err := c.List(context.Background(), "/Movies"); err == nil {
		t.Fatal("timeout must fail")
	}
}

func TestSHA1PriorityAndFallback(t *testing.T) {
	good := strings.Repeat("a", 40)
	srv := &fakeServer{getHandler: func(path string) (any, int, int) {
		switch path {
		case "/structured":
			return map[string]any{"name": "A.mkv", "is_dir": false, "size": 1, "hash_info": map[string]any{"sha1": good}}, 200, 200
		case "/legacy":
			return map[string]any{"name": "A.mkv", "is_dir": false, "size": 1, "hash_info": nil,
				"hashinfo": fmt.Sprintf(`{"sha1":%q}`, good)}, 200, 200
		case "/invalid":
			return map[string]any{"name": "A.mkv", "is_dir": false, "size": 1, "hash_info": map[string]any{"sha1": "nope"}}, 200, 200
		default:
			return map[string]any{"name": "A.mkv", "is_dir": false, "size": 0, "hash_info": nil}, 200, 200
		}
	}}
	ts := srv.serve()
	defer ts.Close()
	c := New(ts.URL, "t")
	ctx := context.Background()

	entry, err := c.Get(ctx, "/structured")
	if err != nil || !entry.HasSHA || entry.SHA1 != good {
		t.Fatalf("structured: %+v %v", entry, err)
	}
	entry, err = c.Get(ctx, "/legacy")
	if err != nil || !entry.HasSHA || entry.SHA1 != good {
		t.Fatalf("legacy: %+v %v", entry, err)
	}
	entry, err = c.Get(ctx, "/invalid")
	if err != nil || entry.HasSHA {
		t.Fatalf("invalid hash must be omitted: %+v %v", entry, err)
	}
	entry, err = c.Get(ctx, "/none")
	if err != nil || entry.HasSHA || entry.Size == nil || *entry.Size != 0 {
		t.Fatalf("zero size must be retained with no hash: %+v %v", entry, err)
	}
}

func TestGetMissingIsDirFails(t *testing.T) {
	srv := &fakeServer{getHandler: func(path string) (any, int, int) {
		return map[string]any{"name": "A.mkv", "size": 1}, 200, 200
	}}
	ts := srv.serve()
	defer ts.Close()
	if _, err := New(ts.URL, "t").Get(context.Background(), "/A.mkv"); err == nil {
		t.Fatal("missing is_dir must fail")
	}
}

func TestContextCancellation(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/fs/list", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	ctx, cancel := context.WithCancel(context.Background())
	var calls int32
	go func() {
		for atomic.LoadInt32(&calls) == 0 {
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()
	atomic.StoreInt32(&calls, 1)
	if _, err := New(ts.URL, "t").List(ctx, "/Movies"); err == nil {
		t.Fatal("cancellation must fail")
	}
}
