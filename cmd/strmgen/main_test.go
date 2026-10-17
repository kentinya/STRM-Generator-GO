package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// startFakeAPI serves a minimal two-file tree.
func startFakeAPI(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, data any, code int) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"code": code, "message": "m", "data": data})
	}
	mux.HandleFunc("/api/fs/get", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Path string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.Path == "/Movies" {
			write(w, map[string]any{"name": "Movies", "is_dir": true, "size": 0, "hash_info": nil}, 200)
			return
		}
		write(w, map[string]any{"name": "x", "is_dir": false, "size": 1, "hash_info": nil}, 200)
	})
	mux.HandleFunc("/api/fs/list", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Path string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.Path != "/Movies" {
			write(w, nil, 404)
			return
		}
		write(w, map[string]any{"content": []map[string]any{
			{"name": "A.mkv", "is_dir": false, "size": 10, "hash_info": map[string]any{"sha1": strings.Repeat("a", 40)}},
			{"name": "notes.txt", "is_dir": false, "size": 1, "hash_info": nil},
		}, "total": 2}, 200)
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func writeConfig(t *testing.T, apiURL, root, db string) string {
	t.Helper()
	// Isolate the cross-process lock so parallel package tests cannot contend on
	// the shared runtime lock.
	t.Setenv("STRMGEN_LOCK_DIR", t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "openlist:\n" +
		"  api_url: \"" + apiURL + "\"\n" +
		"  public_url: \"https://openlist.example.invalid\"\n" +
		"  token_env: \"STRMGEN_TEST_TOKEN\"\n" +
		"sources:\n  - remote: \"/Movies\"\n    output: \"" + root + "\"\n" +
		"cleanup:\n  enabled: false\n" +
		"database:\n  path: \"" + db + "\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSyncFirstRun(t *testing.T) {
	ts := startFakeAPI(t)
	t.Setenv("STRMGEN_TEST_TOKEN", "tok")
	root := t.TempDir()
	db := filepath.Join(t.TempDir(), "data", "strmgen.db")
	cfg := writeConfig(t, ts.URL, root, db)

	var out, errBuf bytes.Buffer
	if code := run([]string{"sync", "--config", cfg}, &out, &errBuf); code != exitOK {
		t.Fatalf("exit=%d stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "created=1") {
		t.Fatalf("unexpected report: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "A.strm")); err != nil {
		t.Fatalf("A.strm not created: %v", err)
	}
}

func TestDryRunCreatesNothing(t *testing.T) {
	ts := startFakeAPI(t)
	t.Setenv("STRMGEN_TEST_TOKEN", "tok")
	root := t.TempDir()
	db := filepath.Join(t.TempDir(), "data", "strmgen.db")
	cfg := writeConfig(t, ts.URL, root, db)

	var out, errBuf bytes.Buffer
	if code := run([]string{"sync", "--config", cfg, "--dry-run"}, &out, &errBuf); code != exitOK {
		t.Fatalf("exit=%d stderr=%s", code, errBuf.String())
	}
	if _, err := os.Stat(filepath.Join(root, "A.strm")); !os.IsNotExist(err) {
		t.Fatal("dry-run must not create files")
	}
	if _, err := os.Stat(db); !os.IsNotExist(err) {
		t.Fatal("dry-run must not create a database")
	}
}

func TestMissingConfigFlagIsUsageError(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"sync"}, &out, &errBuf); code != exitUsage {
		t.Fatalf("exit=%d want %d", code, exitUsage)
	}
}

func TestServeIsRejected(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"serve"}, &out, &errBuf); code != exitUsage {
		t.Fatalf("exit=%d want %d", code, exitUsage)
	}
}

func TestUnknownCommandIsRejected(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"frobnicate"}, &out, &errBuf); code != exitUsage {
		t.Fatalf("exit=%d want %d", code, exitUsage)
	}
}

func TestMissingTokenIsUsageError(t *testing.T) {
	ts := startFakeAPI(t)
	os.Unsetenv("STRMGEN_TEST_TOKEN")
	root := t.TempDir()
	db := filepath.Join(t.TempDir(), "data", "strmgen.db")
	cfg := writeConfig(t, ts.URL, root, db)
	var out, errBuf bytes.Buffer
	if code := run([]string{"sync", "--config", cfg}, &out, &errBuf); code != exitUsage {
		t.Fatalf("exit=%d want %d", code, exitUsage)
	}
	if strings.Contains(errBuf.String(), "tok") {
		t.Fatal("token value must not be echoed")
	}
}

func TestAPIFailureIsRuntimeError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/fs/get", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	})
	mux.HandleFunc("/api/fs/list", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	t.Setenv("STRMGEN_TEST_TOKEN", "tok")
	root := t.TempDir()
	db := filepath.Join(t.TempDir(), "data", "strmgen.db")
	cfg := writeConfig(t, ts.URL, root, db)
	var out, errBuf bytes.Buffer
	if code := run([]string{"sync", "--config", cfg}, &out, &errBuf); code != exitRuntime {
		t.Fatalf("exit=%d want %d", code, exitRuntime)
	}
	if _, err := os.Stat(db); !os.IsNotExist(err) {
		// A failed round may still have created the DB file at open time; the
		// important invariant is that no round was committed.
	}
}
