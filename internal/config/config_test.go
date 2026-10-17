package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const validBase = `
openlist:
  api_url: "http://127.0.0.1:5244"
  public_url: "https://openlist.example.invalid"
  token_env: "OPENLIST_TOKEN"
sources:
  - remote: "/Movies"
    output: "out/Movies"
cleanup:
  enabled: false
database:
  path: "data/strmgen.db"
`

func TestLoadValid(t *testing.T) {
	path := write(t, validBase)
	r, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if r.APIURL != "http://127.0.0.1:5244" {
		t.Fatalf("api url: %q", r.APIURL)
	}
	if !filepath.IsAbs(r.Sources[0].Output) {
		t.Fatalf("output must be absolute: %q", r.Sources[0].Output)
	}
	if r.DirectoryWorkers != DefaultDirectoryWorkers {
		t.Fatalf("default workers: %d", r.DirectoryWorkers)
	}
}

func TestUnknownFieldRejected(t *testing.T) {
	path := write(t, validBase+"extra_field: 1\n")
	if _, err := Load(path); err == nil {
		t.Fatal("unknown top-level field must be rejected")
	}
}

func TestCleanupEnabledRejected(t *testing.T) {
	path := write(t, strings.Replace(validBase, "enabled: false", "enabled: true", 1))
	if _, err := Load(path); err == nil {
		t.Fatal("cleanup.enabled=true must be rejected")
	}
}

func TestAtomicWriteCannotBeDisabled(t *testing.T) {
	path := write(t, validBase+"sync:\n  atomic_write: false\n")
	if _, err := Load(path); err == nil {
		t.Fatal("atomic_write=false must be rejected")
	}
}

func TestOverlappingOutputsRejected(t *testing.T) {
	body := strings.Replace(validBase, "  - remote: \"/Movies\"\n    output: \"out/Movies\"\n",
		"  - remote: \"/Movies\"\n    output: \"out\"\n  - remote: \"/TV\"\n    output: \"out/TV\"\n", 1)
	path := write(t, body)
	if _, err := Load(path); err == nil {
		t.Fatal("overlapping outputs must be rejected")
	}
}

func TestDuplicateOutputsRejected(t *testing.T) {
	body := strings.Replace(validBase, "  - remote: \"/Movies\"\n    output: \"out/Movies\"\n",
		"  - remote: \"/Movies\"\n    output: \"out\"\n  - remote: \"/TV\"\n    output: \"out\"\n", 1)
	path := write(t, body)
	if _, err := Load(path); err == nil {
		t.Fatal("duplicate outputs must be rejected")
	}
}

func TestDatabaseInsideOutputRejected(t *testing.T) {
	body := strings.Replace(validBase, "path: \"data/strmgen.db\"", "path: \"out/Movies/db.sqlite\"", 1)
	path := write(t, body)
	if _, err := Load(path); err == nil {
		t.Fatal("database inside output must be rejected")
	}
}

func TestQueryInPublicURLRejected(t *testing.T) {
	for _, value := range []string{"https://e.invalid?sign=x", "https://e.invalid#frag", "https://u:p@e.invalid", "ftp://e.invalid"} {
		body := strings.Replace(validBase, "\"https://openlist.example.invalid\"", "\""+value+"\"", 1)
		path := write(t, body)
		if _, err := Load(path); err == nil {
			t.Fatalf("public_url %q must be rejected", value)
		}
	}
}

func TestRemoteTraversalRejected(t *testing.T) {
	for _, value := range []string{"Movies", "/Movies/../etc", "/Movies//A", "/Movies/./A"} {
		body := strings.Replace(validBase, "\"/Movies\"", "\""+value+"\"", 1)
		path := write(t, body)
		if _, err := Load(path); err == nil {
			t.Fatalf("remote %q must be rejected", value)
		}
	}
}

func TestTokenRequiresEnv(t *testing.T) {
	path := write(t, validBase)
	r, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("OPENLIST_TOKEN")
	if _, err := r.Token(); err == nil {
		t.Fatal("missing env must error")
	}
	t.Setenv("OPENLIST_TOKEN", "secret")
	got, err := r.Token()
	if err != nil || got != "secret" {
		t.Fatalf("token: %q %v", got, err)
	}
}

func TestSecondDocumentRejected(t *testing.T) {
	path := write(t, validBase+"---\nopenlist:\n  api_url: x\n")
	if _, err := Load(path); err == nil {
		t.Fatal("multiple YAML documents must be rejected")
	}
}
