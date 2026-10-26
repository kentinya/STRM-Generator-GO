package config

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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
	if !r.Mirror || r.CleanupEnabled || r.MissingConfirmations != 3 {
		t.Fatalf("defaults: mirror=%v cleanup=%v confirmations=%d", r.Mirror, r.CleanupEnabled, r.MissingConfirmations)
	}
}

func TestUnknownFieldRejected(t *testing.T) {
	path := write(t, validBase+"extra_field: 1\n")
	if _, err := Load(path); err == nil {
		t.Fatal("unknown top-level field must be rejected")
	}
}

func TestCleanupAndMirrorConfiguration(t *testing.T) {
	withCleanup := func(extra string) string {
		return strings.Replace(validBase, "  enabled: false", "  enabled: true"+extra, 1)
	}
	r, err := Load(write(t, withCleanup("")))
	if err != nil {
		t.Fatalf("explicit cleanup must load: %v", err)
	}
	if !r.CleanupEnabled || r.MissingConfirmations != 3 {
		t.Fatalf("cleanup resolved state: enabled=%v confirmations=%d", r.CleanupEnabled, r.MissingConfirmations)
	}

	for _, value := range []string{"2", "0", "-1"} {
		body := withCleanup("\n  missing_confirmations: " + value)
		if _, err := Load(write(t, body)); err == nil {
			t.Fatalf("missing_confirmations=%s must be rejected", value)
		}
	}
	body := withCleanup("\n  missing_confirmations: 4")
	r, err = Load(write(t, body))
	if err != nil {
		t.Fatalf("higher threshold must load: %v", err)
	}
	if r.MissingConfirmations != 4 {
		t.Fatalf("threshold = %d, want 4", r.MissingConfirmations)
	}

	lowThresholdDisabled := strings.Replace(validBase, "  enabled: false", "  enabled: false\n  missing_confirmations: 2", 1)
	if _, err := Load(write(t, lowThresholdDisabled)); err == nil {
		t.Fatal("a below-minimum threshold must be rejected even when cleanup is disabled")
	}

	mirrorOff := validBase + "sync:\n  mirror: false\n"
	r, err = Load(write(t, mirrorOff))
	if err != nil {
		t.Fatalf("mirror=false without cleanup must load: %v", err)
	}
	if r.Mirror {
		t.Fatal("mirror=false was not resolved")
	}

	contradiction := withCleanup("") + "sync:\n  mirror: false\n"
	_, err = Load(write(t, contradiction))
	if err == nil {
		t.Fatal("cleanup=true with mirror=false must be rejected")
	}
	if diag := SafeDiagnostic(err); !strings.Contains(diag, "cleanup.enabled") {
		t.Fatalf("contradiction diagnostic = %q", diag)
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

func TestOutputAncestorSymlinkRejected(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(validBase, "out/Movies", filepath.Join(alias, "out"), 1)
	path := filepath.Join(base, "case.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("output path through a symlinked ancestor must be rejected")
	}
}

func TestDatabaseAncestorSymlinkRejected(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(validBase, "data/strmgen.db", filepath.Join(alias, "data", "strmgen.db"), 1)
	path := filepath.Join(base, "case.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("database path through a symlinked ancestor must be rejected")
	}
}

func TestOutputRootsCannotOverlapThroughPhysicalAlias(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(validBase, "  - remote: \"/Movies\"\n    output: \"out/Movies\"\n",
		"  - remote: \"/Movies\"\n    output: \""+filepath.Join(real, "out")+"\"\n  - remote: \"/TV\"\n    output: \""+filepath.Join(alias, "out")+"\"\n", 1)
	path := filepath.Join(base, "case.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("lexically distinct roots resolving through an alias must be rejected")
	}
}

func TestRelativePathsUseWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "cwd")
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "case.yaml")
	if err := os.WriteFile(configPath, []byte(validBase), 0o600); err != nil {
		t.Fatal(err)
	}
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	resolved, err := Load(configPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if resolved.Sources[0].Output != filepath.Join(cwd, "out", "Movies") {
		t.Fatalf("relative output resolved against config directory: %q", resolved.Sources[0].Output)
	}
	if resolved.DatabasePath != filepath.Join(cwd, "data", "strmgen.db") {
		t.Fatalf("relative database resolved against config directory: %q", resolved.DatabasePath)
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

func TestIntervalMinutesValidation(t *testing.T) {
	// Omitted -> default 60 and a checked duration.
	r, err := Load(write(t, validBase))
	if err != nil {
		t.Fatalf("load default interval: %v", err)
	}
	if r.IntervalMinutes != DefaultIntervalMinutes || r.Interval != 60*time.Minute {
		t.Fatalf("default interval: minutes=%d duration=%v", r.IntervalMinutes, r.Interval)
	}

	// Explicit positive value is honored.
	body := validBase + "sync:\n  interval_minutes: 5\n"
	r, err = Load(write(t, body))
	if err != nil {
		t.Fatalf("load explicit interval: %v", err)
	}
	if r.IntervalMinutes != 5 || r.Interval != 5*time.Minute {
		t.Fatalf("explicit interval: minutes=%d duration=%v", r.IntervalMinutes, r.Interval)
	}

	// A negative value is invalid for every command and is rejected at load.
	for _, value := range []string{"-1", "-60"} {
		body := validBase + "sync:\n  interval_minutes: " + value + "\n"
		if _, err := Load(write(t, body)); err == nil {
			t.Fatalf("interval_minutes=%s must be rejected at load", value)
		}
	}

	// A value outside the Int64 range cannot even be parsed.
	body = validBase + "sync:\n  interval_minutes: 9223372036854775808\n"
	if _, err := Load(write(t, body)); err == nil {
		t.Fatal("interval_minutes above Int64 must be rejected at load")
	}

	// Zero is accepted by Load for one-shot `sync`, which never schedules; it
	// must be rejected only by the serve-only ValidateServeInterval.
	body = validBase + "sync:\n  interval_minutes: 0\n"
	r, err = Load(write(t, body))
	if err != nil {
		t.Fatalf("sync must accept interval_minutes: 0: %v", err)
	}
	if r.IntervalMinutes != 0 || r.Interval != 0 {
		t.Fatalf("zero interval resolved to minutes=%d duration=%v", r.IntervalMinutes, r.Interval)
	}
	if err := r.ValidateServeInterval(); err == nil {
		t.Fatal("serve must reject interval_minutes: 0")
	}

	// A value that overflows the minutes-to-Duration multiplication is
	// accepted by Load (one-shot `sync` ignores it) but rejected for serve.
	maxSafe := math.MaxInt64 / int64(time.Minute)
	if maxSafe > math.MaxInt64-1 {
		t.Skip("int cannot hold the overflow fixture on this platform")
	}
	overflowMinutes := maxSafe + 1
	if overflowMinutes <= math.MaxInt64 {
		body := validBase + "sync:\n  interval_minutes: " + strconv.FormatInt(overflowMinutes, 10) + "\n"
		r, err = Load(write(t, body))
		if err != nil {
			t.Fatalf("overflow interval must load for sync: %v", err)
		}
		if r.Interval > 0 {
			t.Fatalf("overflow interval produced a positive duration: %v", r.Interval)
		}
		if err := r.ValidateServeInterval(); err == nil {
			t.Fatalf("serve must reject overflow interval_minutes=%d", overflowMinutes)
		}
	}

	// The exact safe maximum still converts to a positive duration and passes
	// serve validation.
	body = validBase + "sync:\n  interval_minutes: " + strconv.FormatInt(maxSafe, 10) + "\n"
	r, err = Load(write(t, body))
	if err != nil {
		t.Fatalf("safe maximum interval must load: %v", err)
	}
	if r.Interval <= 0 {
		t.Fatalf("safe maximum interval produced a non-positive duration: %v", r.Interval)
	}
	if err := r.ValidateServeInterval(); err != nil {
		t.Fatalf("safe maximum interval must pass serve validation: %v", err)
	}
}

func TestIntervalMinutesMustBeInteger(t *testing.T) {
	for _, value := range []string{"\"60\"", "1.5", "true"} {
		body := validBase + "sync:\n  interval_minutes: " + value + "\n"
		if _, err := Load(write(t, body)); err == nil {
			t.Fatalf("non-integer interval_minutes=%s must be rejected", value)
		}
	}
}

func TestIntervalDiagnosticIsSafe(t *testing.T) {
	const secret = "SYNTHETIC_INTERVAL_SECRET_MARKER"
	body := validBase + "sync:\n  interval_minutes: \"" + secret + "\"\n"
	err := func() error { _, err := Load(write(t, body)); return err }()
	if err == nil {
		t.Fatal("non-integer interval must be rejected")
	}
	diag := SafeDiagnostic(err)
	if strings.Contains(diag, secret) {
		t.Fatalf("interval diagnostic leaked a value: %q", diag)
	}
	if diag != "invalid configuration" {
		t.Fatalf("interval type diagnostic should stay a fixed safe reason: %q", diag)
	}

	// A negative interval is rejected by the semantic check, whose fixed reason
	// names the trusted field without echoing any configured value.
	body = validBase + "sync:\n  interval_minutes: -5\n"
	err = func() error { _, err := Load(write(t, body)); return err }()
	if err == nil {
		t.Fatal("negative interval must be rejected")
	}
	if diag := SafeDiagnostic(err); !strings.Contains(diag, "sync.interval_minutes") {
		t.Fatalf("negative interval diagnostic should name the field: %q", diag)
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

// withExtensions inserts one raw sync block carrying a media_extensions value.
func withExtensions(raw string) string {
	return strings.Replace(validBase, "cleanup:\n", "sync:\n"+raw+"\ncleanup:\n", 1)
}

func TestMediaExtensionsDefaultIsUnchanged(t *testing.T) {
	r, err := Load(write(t, validBase))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := []string{".mkv", ".mp4", ".m4v", ".avi", ".mov", ".wmv", ".webm", ".ts", ".m2ts", ".mpg", ".mpeg"}
	if len(r.MediaExtensions) != len(want) {
		t.Fatalf("default list = %v, want %v", r.MediaExtensions, want)
	}
	for i := range want {
		if r.MediaExtensions[i] != want[i] {
			t.Fatalf("default list = %v, want %v", r.MediaExtensions, want)
		}
	}
	// A caller must not be able to corrupt the default through the resolved copy.
	r.MediaExtensions[0] = ".zzz"
	again, err := Load(write(t, validBase))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if again.MediaExtensions[0] != ".mkv" {
		t.Fatalf("default list was mutated through a resolved copy: %v", again.MediaExtensions)
	}
}

func TestMediaExtensionsExplicitListReplacesAndNormalises(t *testing.T) {
	// A YAML flow sequence is the documented spelling; a block sequence and
	// quoted values are covered by the table below.
	body := withExtensions("  media_extensions: [\" .MKV \", \"FLV\", \".mp4\", \".mkv\", \"Mkv\", \".MP3\"]")
	r, err := Load(write(t, body))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := []string{".mkv", ".flv", ".mp4", ".mp3"}
	if len(r.MediaExtensions) != len(want) {
		t.Fatalf("resolved list = %v, want %v", r.MediaExtensions, want)
	}
	for i := range want {
		if r.MediaExtensions[i] != want[i] {
			t.Fatalf("resolved list = %v, want %v", r.MediaExtensions, want)
		}
	}
	for _, absent := range []string{".avi", ".mov", ".webm"} {
		for _, got := range r.MediaExtensions {
			if got == absent {
				t.Fatalf("explicit list merged a default extension: %v", r.MediaExtensions)
			}
		}
	}
}

func TestMediaExtensionsBlockSequenceAccepted(t *testing.T) {
	body := withExtensions("  media_extensions:\n    - \".flv\"\n    - \"MKV\"\n")
	r, err := Load(write(t, body))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(r.MediaExtensions) != 2 || r.MediaExtensions[0] != ".flv" || r.MediaExtensions[1] != ".mkv" {
		t.Fatalf("resolved list = %v", r.MediaExtensions)
	}
}

func TestMediaExtensionsRejectInvalidInput(t *testing.T) {
	cases := map[string]string{
		"empty flow list":       "  media_extensions: []",
		"empty block list":      "  media_extensions:\n",
		"null":                  "  media_extensions: null",
		"tilde null":            "  media_extensions: ~",
		"single string":         "  media_extensions: \".mkv\"",
		"number":                "  media_extensions: 5",
		"boolean":               "  media_extensions: [true]",
		"integer element":       "  media_extensions: [5]",
		"nested sequence":       "  media_extensions: [[\".mkv\"]]",
		"nested mapping":        "  media_extensions: [{a: b}]",
		"null element":          "  media_extensions: [null]",
		"empty item":            "  media_extensions: [\"\"]",
		"blank item":            "  media_extensions: [\"   \"]",
		"bare dot":              "  media_extensions: [\".\"]",
		"double dot":            "  media_extensions: [\"..\"]",
		"multi-segment suffix":  "  media_extensions: [\".tar.gz\"]",
		"path":                  "  media_extensions: [\"/etc/passwd\"]",
		"relative path":         "  media_extensions: [\"media/mkv\"]",
		"wildcard":              "  media_extensions: [\"*.mkv\"]",
		"internal whitespace":   "  media_extensions: [\"m kv\"]",
		"non-ascii letters":     "  media_extensions: [\".m\u00e4kv\"]",
		"strm output suffix":    "  media_extensions: [\".strm\"]",
		"upper case strm":       "  media_extensions: [\"STRM\"]",
		"one valid one invalid": "  media_extensions: [\".mkv\", \"a.b\"]",
	}
	for name, raw := range cases {
		if _, err := Load(write(t, withExtensions(raw))); err == nil {
			t.Fatalf("%s must be rejected", name)
		} else if diag := SafeDiagnostic(err); !strings.Contains(diag, "sync.media_extensions") {
			t.Fatalf("%s diagnostic = %q", name, diag)
		}
	}
}

func TestMediaExtensionsDiagnosticDoesNotEchoInput(t *testing.T) {
	marker := "PRIVATE_EXTENSION_MARKER_20261026"
	for _, raw := range []string{
		"  media_extensions: [\"/private/" + marker + ".mkv\"]",
		"  media_extensions: [\"" + strings.Repeat(marker, 4) + "\"]",
	} {
		_, err := Load(write(t, withExtensions(raw)))
		if err == nil {
			t.Fatalf("marker input must be rejected: %s", raw)
		}
		if strings.Contains(SafeDiagnostic(err), marker) {
			t.Fatalf("diagnostic echoed the configured value: %q", SafeDiagnostic(err))
		}
	}
}

func TestMediaExtensionsUnknownFieldStillRejected(t *testing.T) {
	body := strings.Replace(validBase, "cleanup:\n", "sync:\n  media_extension: [\".mkv\"]\ncleanup:\n", 1)
	if _, err := Load(write(t, body)); err == nil {
		t.Fatal("a misspelled extension field must still be rejected")
	}
}
