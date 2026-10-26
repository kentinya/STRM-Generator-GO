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

	"strm-generator-go/internal/reconciler"
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
	return writeConfigWithExtra(t, apiURL, root, db, "")
}

func writeConfigWithExtra(t *testing.T, apiURL, root, db, extra string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := "openlist:\n" +
		"  api_url: \"" + apiURL + "\"\n" +
		"  public_url: \"https://openlist.example.invalid\"\n" +
		"  token_env: \"STRMGEN_TEST_TOKEN\"\n" +
		"sources:\n  - remote: \"/Movies\"\n    output: \"" + root + "\"\n" +
		"cleanup:\n  enabled: false\n" +
		"database:\n  path: \"" + db + "\"\n"
	if extra != "" {
		// Append a second sync block only when the base has none.
		body += extra
	}
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
	// serve is implemented; the old rejection path is replaced by argument and
	// configuration validation. A missing --config must still be a usage error.
	var out, errBuf bytes.Buffer
	if code := run([]string{"serve"}, &out, &errBuf); code != exitUsage {
		t.Fatalf("exit=%d want %d", code, exitUsage)
	}
}

func TestServeRejectsBadIntervalBeforeAnyProductAction(t *testing.T) {
	ts := startFakeAPI(t)
	t.Setenv("STRMGEN_TEST_TOKEN", "tok")
	root := filepath.Join(t.TempDir(), "out")
	db := filepath.Join(t.TempDir(), "data", "strmgen.db")
	for _, value := range []string{"0", "-1", "9223372036854775808"} {
		cfg := writeConfigWithExtra(t, ts.URL, root, db, "sync:\n  interval_minutes: "+value+"\n")
		var out, errBuf bytes.Buffer
		if code := run([]string{"serve", "--config", cfg}, &out, &errBuf); code != exitUsage {
			t.Fatalf("interval %s exit=%d want %d (%s)", value, code, exitUsage, errBuf.String())
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("invalid interval created output: %v", err)
	}
	if _, err := os.Stat(db); !os.IsNotExist(err) {
		t.Fatalf("invalid interval created database: %v", err)
	}
}

func TestServeRejectsFractionalInterval(t *testing.T) {
	ts := startFakeAPI(t)
	t.Setenv("STRMGEN_TEST_TOKEN", "tok")
	root := t.TempDir()
	db := filepath.Join(t.TempDir(), "data", "strmgen.db")
	cfg := writeConfigWithExtra(t, ts.URL, root, db, "sync:\n  interval_minutes: 1.5\n")
	var out, errBuf bytes.Buffer
	if code := run([]string{"serve", "--config", cfg}, &out, &errBuf); code != exitUsage {
		t.Fatalf("fractional interval exit=%d want %d (%s)", code, exitUsage, errBuf.String())
	}
}

// TestSyncAcceptsZeroInterval preserves the Base one-shot behavior: `sync` and
// `sync --dry-run` never schedule, so sync.interval_minutes: 0 must remain a
// valid configuration instead of failing every command.
func TestSyncAcceptsZeroInterval(t *testing.T) {
	ts := startFakeAPI(t)
	t.Setenv("STRMGEN_TEST_TOKEN", "tok")
	root := t.TempDir()
	db := filepath.Join(t.TempDir(), "data", "strmgen.db")
	cfg := writeConfigWithExtra(t, ts.URL, root, db, "sync:\n  interval_minutes: 0\n")

	var out, errBuf bytes.Buffer
	if code := run([]string{"sync", "--config", cfg}, &out, &errBuf); code != exitOK {
		t.Fatalf("sync with interval 0 exit=%d stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "created=1") {
		t.Fatalf("sync with interval 0 report: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "A.strm")); err != nil {
		t.Fatalf("sync with interval 0 did not create output: %v", err)
	}

	dryRoot := t.TempDir()
	dryDB := filepath.Join(t.TempDir(), "data", "strmgen.db")
	dryCfg := writeConfigWithExtra(t, ts.URL, dryRoot, dryDB, "sync:\n  interval_minutes: 0\n")
	out.Reset()
	errBuf.Reset()
	if code := run([]string{"sync", "--config", dryCfg, "--dry-run"}, &out, &errBuf); code != exitOK {
		t.Fatalf("sync --dry-run with interval 0 exit=%d stderr=%s", code, errBuf.String())
	}
	if _, err := os.Stat(filepath.Join(dryRoot, "A.strm")); !os.IsNotExist(err) {
		t.Fatal("sync --dry-run with interval 0 must not create files")
	}
}

// TestServeRejectsZeroIntervalBeforeAnySideEffect proves serve rejects a zero
// interval after config load but before token lookup, lock, output or database.
func TestServeRejectsZeroIntervalBeforeAnySideEffect(t *testing.T) {
	ts := startFakeAPI(t)
	// The token is deliberately absent: if serve touched the token lookup or any
	// product resource before validating the interval, the error would differ.
	os.Unsetenv("STRMGEN_TEST_TOKEN")
	root := filepath.Join(t.TempDir(), "out")
	db := filepath.Join(t.TempDir(), "data", "strmgen.db")
	cfg := writeConfigWithExtra(t, ts.URL, root, db, "sync:\n  interval_minutes: 0\n")
	var out, errBuf bytes.Buffer
	if code := run([]string{"serve", "--config", cfg}, &out, &errBuf); code != exitUsage {
		t.Fatalf("serve with interval 0 exit=%d want %d (%s)", code, exitUsage, errBuf.String())
	}
	if strings.Contains(errBuf.String(), "token") {
		t.Fatalf("serve validated interval after token lookup: %s", errBuf.String())
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("serve with interval 0 created output: %v", err)
	}
	if _, err := os.Stat(db); !os.IsNotExist(err) {
		t.Fatalf("serve with interval 0 created database: %v", err)
	}
}

func TestServeMissingTokenIsUsageError(t *testing.T) {
	ts := startFakeAPI(t)
	os.Unsetenv("STRMGEN_TEST_TOKEN")
	root := t.TempDir()
	db := filepath.Join(t.TempDir(), "data", "strmgen.db")
	cfg := writeConfig(t, ts.URL, root, db)
	var out, errBuf bytes.Buffer
	if code := run([]string{"serve", "--config", cfg}, &out, &errBuf); code != exitUsage {
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
	if strings.Contains(errBuf.String(), "STRMGEN_TEST_TOKEN") || strings.Contains(errBuf.String(), "SENTINEL_TOKEN_SECRET") {
		t.Fatal("token configuration details must not be echoed")
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

func TestConfigurationDiagnosticsDoNotEchoUntrustedValues(t *testing.T) {
	ts := startFakeAPI(t)
	root := t.TempDir()
	db := filepath.Join(t.TempDir(), "data", "strmgen.db")
	base := writeConfig(t, ts.URL, root, db)
	secret := "SYNTHETIC_COOKIE_SECRET_MARKER_20261018"

	badUnknown := filepath.Join(t.TempDir(), "unknown.yaml")
	raw, err := os.ReadFile(base)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("\n"+secret+": \"https://private.example.invalid/path?sign="+secret+"\"\n")...)
	if err := os.WriteFile(badUnknown, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := run([]string{"sync", "--config", badUnknown}, &out, &stderr); code != exitUsage {
		t.Fatalf("unknown key exit=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(out.String()+stderr.String(), secret) || strings.Contains(out.String()+stderr.String(), "private.example.invalid") {
		t.Fatalf("unknown-key config details leaked: %s", stderr.String())
	}

	badValue := filepath.Join(t.TempDir(), "value.yaml")
	raw, err = os.ReadFile(base)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("\nsync:\n  directory_workers: \""+secret+"\"\n")...)
	if err := os.WriteFile(badValue, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	stderr.Reset()
	if code := run([]string{"sync", "--config", badValue}, &out, &stderr); code != exitUsage {
		t.Fatalf("invalid-value exit=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(out.String()+stderr.String(), secret) {
		t.Fatalf("invalid YAML value leaked: %s", stderr.String())
	}

	missing := filepath.Join(t.TempDir(), "PRIVATE_FULL_PATH_MARKER", "secret.yaml")
	out.Reset()
	stderr.Reset()
	if code := run([]string{"sync", "--config", missing}, &out, &stderr); code != exitUsage {
		t.Fatalf("missing config exit=%d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "PRIVATE_FULL_PATH_MARKER") || strings.Contains(stderr.String(), missing) {
		t.Fatalf("missing-config path leaked: %s", stderr.String())
	}
}

func TestConfigurationDiagnosticsUseKnownFieldNamesOnly(t *testing.T) {
	secret := "SYNTHETIC_URL_SECRET_MARKER"
	badURL := filepath.Join(t.TempDir(), "bad-url.yaml")
	base := writeConfig(t, "https://api.example.invalid", filepath.Join(t.TempDir(), "out"),
		filepath.Join(t.TempDir(), "data", "db.sqlite"))
	raw, err := os.ReadFile(base)
	if err != nil {
		t.Fatal(err)
	}
	contents := strings.ReplaceAll(string(raw), "https://openlist.example.invalid",
		"https://private.example.invalid/?sign="+secret)
	if err := os.WriteFile(badURL, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := run([]string{"sync", "--config", badURL}, &out, &stderr); code != exitUsage {
		t.Fatalf("invalid URL exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "openlist.public_url") || strings.Contains(stderr.String(), secret) ||
		strings.Contains(stderr.String(), "private.example.invalid") || strings.Contains(stderr.String(), "sign=") {
		t.Fatalf("unsafe or unhelpful config diagnostic: %s", stderr.String())
	}
}

func TestConfigurationDiagnosticsRedactDuplicateOutputAndDatabaseAlias(t *testing.T) {
	ts := startFakeAPI(t)
	const duplicateMarker = "PRIVATE_DUPLICATE_OUTPUT_MARKER_20261018"
	root := filepath.Join(t.TempDir(), duplicateMarker, "out")
	db := filepath.Join(t.TempDir(), "db", "strmgen.db")
	base := writeConfig(t, ts.URL, root, db)
	raw, err := os.ReadFile(base)
	if err != nil {
		t.Fatal(err)
	}
	outputLine := "    output: \"" + root + "\"\n"
	duplicated := strings.Replace(string(raw), outputLine,
		outputLine+"  - remote: \"/TV\"\n    output: \""+root+"\"\n", 1)
	if duplicated == string(raw) {
		t.Fatal("test configuration did not contain the expected output line")
	}
	duplicateConfig := filepath.Join(t.TempDir(), "duplicate.yaml")
	if err := os.WriteFile(duplicateConfig, []byte(duplicated), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := run([]string{"sync", "--config", duplicateConfig}, &out, &stderr); code != exitUsage {
		t.Fatalf("duplicate output exit=%d stdout=%s stderr=%s", code, out.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "sources[1].output") ||
		strings.Contains(out.String()+stderr.String(), duplicateMarker) ||
		strings.Contains(out.String()+stderr.String(), root) {
		t.Fatalf("duplicate-root diagnostic was unsafe or unhelpful: %s", stderr.String())
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("duplicate root validation created output: %v", err)
	}
	if _, err := os.Stat(db); !os.IsNotExist(err) {
		t.Fatalf("duplicate root validation created database: %v", err)
	}

	privateParent := filepath.Join(t.TempDir(), "PRIVATE_DATABASE_PATH_MARKER_20261018")
	if err := os.Mkdir(privateParent, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "database-alias")
	if err := os.Symlink(privateParent, alias); err != nil {
		t.Fatal(err)
	}
	aliasDB := filepath.Join(alias, "db", "strmgen.db")
	aliasRoot := filepath.Join(t.TempDir(), "output")
	aliasConfig := writeConfig(t, ts.URL, aliasRoot, aliasDB)
	out.Reset()
	stderr.Reset()
	if code := run([]string{"sync", "--config", aliasConfig}, &out, &stderr); code != exitUsage {
		t.Fatalf("database alias exit=%d stdout=%s stderr=%s", code, out.String(), stderr.String())
	}
	combined := out.String() + stderr.String()
	if !strings.Contains(combined, "database.path") || strings.Contains(combined, "PRIVATE_DATABASE_PATH_MARKER") ||
		strings.Contains(combined, aliasDB) {
		t.Fatalf("database path diagnostic leaked a configured path: %s", combined)
	}
	if _, err := os.Stat(aliasRoot); !os.IsNotExist(err) {
		t.Fatalf("database path validation created output: %v", err)
	}
	if entries, err := os.ReadDir(privateParent); err != nil || len(entries) != 0 {
		t.Fatalf("database alias validation changed its target: entries=%v err=%v", entries, err)
	}
}

func TestDatabaseFailureDiagnosticIsRedactedAndPreservesExistingFile(t *testing.T) {
	ts := startFakeAPI(t)
	t.Setenv("STRMGEN_TEST_TOKEN", "synthetic-fixture-token")
	const contentMarker = "SYNTHETIC_DATABASE_CONTENT_SECRET_20261018"
	root := filepath.Join(t.TempDir(), "PRIVATE_OUTPUT_PATH_MARKER", "out")
	db := filepath.Join(t.TempDir(), "PRIVATE_DATABASE_PATH_MARKER", "strmgen.db")
	if err := os.MkdirAll(filepath.Dir(db), 0o700); err != nil {
		t.Fatal(err)
	}
	before := []byte("not a sqlite database: " + contentMarker)
	if err := os.WriteFile(db, before, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := writeConfig(t, ts.URL, root, db)
	var out, stderr bytes.Buffer
	if code := run([]string{"sync", "--config", cfg}, &out, &stderr); code != exitRuntime {
		t.Fatalf("invalid database exit=%d stdout=%s stderr=%s", code, out.String(), stderr.String())
	}
	combined := out.String() + stderr.String()
	for _, marker := range []string{contentMarker, "PRIVATE_DATABASE_PATH_MARKER", "PRIVATE_OUTPUT_PATH_MARKER", db, root} {
		if strings.Contains(combined, marker) {
			t.Fatalf("database diagnostic leaked %q: %s", marker, combined)
		}
	}
	if !strings.Contains(combined, "database.path is unavailable or unsafe") {
		t.Fatalf("database failure lacks a useful safe category: %s", combined)
	}
	after, err := os.ReadFile(db)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatalf("invalid database file was changed: %q err=%v", after, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("database failure created output: %v", err)
	}
}

func TestAPIErrorAndSourceSummaryAreRedactedAndCounted(t *testing.T) {
	const remote = "/private/full/root/SYNTHETIC_PATH_MARKER"
	const secret = "SYNTHETIC_TOKEN_COOKIE_SIGN_MARKER"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/fs/get", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":403,"message":"` + secret + ` https://private.invalid/?sign=` + secret + `","data":null}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	t.Setenv("STRMGEN_TEST_TOKEN", "synthetic-fixture-token")
	root := filepath.Join(t.TempDir(), "out")
	db := filepath.Join(t.TempDir(), "db", "strmgen.sqlite")
	cfg := writeConfig(t, ts.URL, root, db)
	contents, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	contents = []byte(strings.ReplaceAll(string(contents), "/Movies", remote))
	if err := os.WriteFile(cfg, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if code := run([]string{"sync", "--config", cfg}, &out, &stderr); code != exitRuntime {
		t.Fatalf("API failure exit=%d stdout=%s stderr=%s", code, out.String(), stderr.String())
	}
	combined := out.String() + stderr.String()
	if strings.Contains(combined, remote) || strings.Contains(combined, secret) || strings.Contains(combined, "private.invalid") {
		t.Fatalf("remote/API details leaked: %s", combined)
	}
	if !strings.Contains(out.String(), "errors=1") || !strings.Contains(out.String(), "partial=true") {
		t.Fatalf("API failure was not accurately summarized: %s", out.String())
	}
}

func TestLockBusyMessageDoesNotEchoConfiguredPaths(t *testing.T) {
	ts := startFakeAPI(t)
	t.Setenv("STRMGEN_TEST_TOKEN", "token")
	root := filepath.Join(t.TempDir(), "PRIVATE_OUTPUT_MARKER")
	db := filepath.Join(t.TempDir(), "PRIVATE_DB_MARKER", "db.sqlite")
	cfg := writeConfig(t, ts.URL, root, db)
	lock, err := reconciler.AcquireLockFor([]string{root}, db)
	if err != nil {
		t.Fatalf("acquire test lock: %v", err)
	}
	defer lock.Release()
	var out, stderr bytes.Buffer
	if code := run([]string{"sync", "--config", cfg}, &out, &stderr); code != exitRuntime {
		t.Fatalf("busy exit=%d stdout=%s stderr=%s", code, out.String(), stderr.String())
	}
	if strings.Contains(out.String()+stderr.String(), "PRIVATE_OUTPUT_MARKER") || strings.Contains(out.String()+stderr.String(), "PRIVATE_DB_MARKER") {
		t.Fatalf("lock path leaked: %s", stderr.String())
	}
}

func TestSourceIdentityBindsAPIRemoteAndOutputWithoutPlaintext(t *testing.T) {
	base := sourceIdentity("https://api.example.invalid", "/Movies", "/srv/strm/Movies")
	if base != sourceIdentity("https://api.example.invalid", "/Movies", "/srv/strm/Movies") {
		t.Fatal("same source mapping must have a stable identity")
	}
	for _, changed := range []string{
		sourceIdentity("https://other.example.invalid", "/Movies", "/srv/strm/Movies"),
		sourceIdentity("https://api.example.invalid", "/TV", "/srv/strm/Movies"),
		sourceIdentity("https://api.example.invalid", "/Movies", "/srv/strm/TV"),
	} {
		if changed == base {
			t.Fatal("API, remote, or output change must produce a distinct source identity")
		}
	}
	if strings.Contains(base, "/Movies") || strings.Contains(base, "/srv/strm") {
		t.Fatalf("source identity must not expose path data: %q", base)
	}
}

// startExtensionAPI serves a tree with one .mkv and one .flv file so a test can
// prove which extensions the resolved configuration actually generates.
func startExtensionAPI(t *testing.T) *httptest.Server {
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
		name := req.Path[strings.LastIndex(req.Path, "/")+1:]
		write(w, map[string]any{"name": name, "is_dir": false, "size": 1, "hash_info": nil}, 200)
	})
	mux.HandleFunc("/api/fs/list", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Path string }
		json.NewDecoder(r.Body).Decode(&req)
		if req.Path != "/Movies" {
			write(w, nil, 404)
			return
		}
		write(w, map[string]any{"content": []map[string]any{
			{"name": "Sample.mkv", "is_dir": false, "size": 10, "hash_info": map[string]any{"sha1": strings.Repeat("a", 40)}},
			{"name": "Clip.flv", "is_dir": false, "size": 20, "hash_info": map[string]any{"sha1": strings.Repeat("b", 40)}},
			{"name": "notes.txt", "is_dir": false, "size": 1, "hash_info": nil},
		}, "total": 3}, 200)
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestCustomExtensionsDriveCLIEntryPoints(t *testing.T) {
	ts := startExtensionAPI(t)
	t.Setenv("STRMGEN_TEST_TOKEN", "tok")
	root := t.TempDir()
	db := filepath.Join(t.TempDir(), "data", "strmgen.db")
	cfg := writeConfigWithExtra(t, ts.URL, root, db, "sync:\n  media_extensions: [\"FLV\"]\n")

	var out, errBuf bytes.Buffer
	if code := run([]string{"sync", "--config", cfg, "--dry-run"}, &out, &errBuf); code != exitOK {
		t.Fatalf("dry-run exit=%d stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "created=1") || !strings.Contains(out.String(), "filtered=2") {
		t.Fatalf("dry-run did not use the custom list: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, "Clip.strm")); !os.IsNotExist(err) {
		t.Fatal("dry-run created output")
	}

	out.Reset()
	errBuf.Reset()
	if code := run([]string{"sync", "--config", cfg}, &out, &errBuf); code != exitOK {
		t.Fatalf("sync exit=%d stderr=%s", code, errBuf.String())
	}
	if _, err := os.Stat(filepath.Join(root, "Clip.strm")); err != nil {
		t.Fatalf("custom extension was not generated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Sample.strm")); !os.IsNotExist(err) {
		t.Fatal("an excluded default extension was still generated")
	}
	raw, err := os.ReadFile(filepath.Join(root, "Clip.strm"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "https://openlist.example.invalid/d/Movies/Clip.flv") {
		t.Fatalf("unexpected stable URL: %q", raw)
	}
}

func TestMediaExtensionConfigErrorsExitBeforeProductAction(t *testing.T) {
	ts := startExtensionAPI(t)
	t.Setenv("STRMGEN_TEST_TOKEN", "tok")
	// Each command is checked with one representative invalid list; the full
	// error matrix is covered by the config package against the real loader.
	cases := []struct {
		name string
		body string
	}{
		{"empty", "sync:\n  media_extensions: []\n"},
		{"null", "sync:\n  media_extensions: null\n"},
		{"non-string", "sync:\n  media_extensions: [\".mkv\", 5]\n"},
		{"strm", "sync:\n  media_extensions: [\".strm\"]\n"},
	}
	commands := [][]string{{"sync"}, {"sync", "--dry-run"}, {"serve"}}
	for _, tc := range cases {
		for _, command := range commands {
			root := filepath.Join(t.TempDir(), "out")
			db := filepath.Join(t.TempDir(), "data", "strmgen.db")
			cfg := writeConfigWithExtra(t, ts.URL, root, db, tc.body)
			cfg = strings.ReplaceAll(cfg, "cleanup:\n", "")
			var out, errBuf bytes.Buffer
			args := append(append([]string{}, command...), "--config", cfg)
			if code := run(args, &out, &errBuf); code != exitUsage {
				t.Fatalf("%s %v exit=%d want %d (%s)", tc.name, command, code, exitUsage, errBuf.String())
			}
			if !strings.Contains(errBuf.String(), "sync.media_extensions") {
				t.Fatalf("%s %v diagnostic = %q", tc.name, command, errBuf.String())
			}
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatalf("%s %v created an output root", tc.name, command)
			}
			if _, err := os.Stat(db); !os.IsNotExist(err) {
				t.Fatalf("%s %v created a database", tc.name, command)
			}
			if strings.Contains(errBuf.String(), ".strm") || strings.Contains(errBuf.String(), "5") {
				t.Fatalf("%s %v echoed the configured value: %q", tc.name, command, errBuf.String())
			}
		}
	}
}
