// Package config parses and validates the strmgen YAML contract before any
// output is produced. It deliberately keeps the set of supported fields small
// and rejects unknown fields so a typo cannot silently disable a safety
// invariant.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"strm-generator-go/internal/securepath"

	"gopkg.in/yaml.v3"
)

// DefaultDirectoryWorkers is used when sync.directory_workers is omitted.
const DefaultDirectoryWorkers = 4

// MaxDirectoryWorkers bounds the per-round directory concurrency.
const MaxDirectoryWorkers = 32

const maxRemotePathBytes = 4096

// OpenList holds API connectivity. token is never stored in this struct.
type OpenList struct {
	APIURL    string `yaml:"api_url"`
	PublicURL string `yaml:"public_url"`
	TokenEnv  string `yaml:"token_env"`
}

// Source maps one remote directory tree onto one local output root.
type Source struct {
	Remote string `yaml:"remote"`
	Output string `yaml:"output"`
}

// Sync holds the safety-relevant sync switches plus future fields that may be
// parsed but must not start background work.
type Sync struct {
	IntervalMinutes  *int  `yaml:"interval_minutes"`
	DirectoryWorkers *int  `yaml:"directory_workers"`
	AtomicWrite      *bool `yaml:"atomic_write"`
	SkipUnchanged    *bool `yaml:"skip_unchanged"`
	Mirror           *bool `yaml:"mirror"`
}

// Cleanup is parsed for forward compatibility; enabling it is rejected.
type Cleanup struct {
	Enabled              *bool `yaml:"enabled"`
	MissingConfirmations *int  `yaml:"missing_confirmations"`
}

// Database points at the SQLite ownership store.
type Database struct {
	Path string `yaml:"path"`
}

// Config is the raw parsed document.
type Config struct {
	OpenList OpenList `yaml:"openlist"`
	Sources  []Source `yaml:"sources"`
	Sync     Sync     `yaml:"sync"`
	Cleanup  Cleanup  `yaml:"cleanup"`
	Database Database `yaml:"database"`
}

// ResolvedSource is a validated source with an absolute output root.
type ResolvedSource struct {
	Index  int
	Remote string
	Output string
}

// Resolved is the validated configuration used by the syncer.
type Resolved struct {
	APIURL               string
	PublicPrefix         string
	TokenEnv             string
	DirectoryWorkers     int
	DatabasePath         string
	Mirror               bool
	CleanupEnabled       bool
	MissingConfirmations int
	Sources              []ResolvedSource
	baseDir              string
}

// Load reads, strictly decodes and validates a YAML file.
func Load(file string) (*Resolved, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, errors.New("cannot read configuration file")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("config file is empty")
	}
	cfg := &Config{}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		// YAML diagnostics can echo arbitrary values. Never reflect raw config.
		return nil, errors.New("configuration syntax or fields are invalid")
	}
	// Reject a second YAML document: the contract is a single document.
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return nil, errors.New("config must contain exactly one YAML document")
	} else if !errors.Is(err, io.EOF) {
		return nil, errors.New("configuration syntax or fields are invalid")
	}
	baseDir, err := os.Getwd()
	if err != nil {
		return nil, errors.New("cannot determine startup working directory")
	}
	return validate(cfg, baseDir)
}

var yamlLineDiagnostic = regexp.MustCompile(`(?:^|\n)(?:yaml: )?line ([0-9]+):`)

// SafeDiagnostic converts an internal config error into a stable, non-secret
// user message. It only reflects known field names and numeric source indexes;
// YAML snippets, input values, and filesystem paths are never returned.
func SafeDiagnostic(err error) string {
	if err == nil {
		return "invalid configuration"
	}
	message := err.Error()
	if match := yamlLineDiagnostic.FindStringSubmatch(message); len(match) == 2 {
		return "invalid configuration at line " + match[1]
	}
	if field := safeFieldFromError(message); field != "" {
		return "invalid " + field
	}
	switch {
	case strings.Contains(message, "configuration file"):
		return "configuration file is unavailable"
	case strings.Contains(message, "working directory"):
		return "startup working directory is unavailable"
	case strings.Contains(message, "exactly one YAML document"):
		return "configuration must contain exactly one YAML document"
	case strings.Contains(message, "at least one source"):
		return "sources must contain at least one entry"
	default:
		return "invalid configuration"
	}
}

func safeFieldFromError(message string) string {
	for _, field := range []string{
		"openlist.api_url", "openlist.public_url", "openlist.token_env",
		"sync.directory_workers", "sync.interval_minutes", "sync.atomic_write", "sync.skip_unchanged", "sync.mirror",
		"cleanup.enabled", "cleanup.missing_confirmations", "database.path",
	} {
		if strings.HasPrefix(message, field+":") || strings.HasPrefix(message, field+" ") {
			return field
		}
	}
	if strings.HasPrefix(message, "database.path or SQLite sidecar") {
		return "database.path"
	}
	if strings.HasPrefix(message, "sources[") {
		close := strings.IndexByte(message, ']')
		if close <= len("sources[") || close+2 >= len(message) || message[close+1] != '.' {
			return ""
		}
		indexText := message[len("sources["):close]
		if _, err := strconv.Atoi(indexText); err != nil {
			return ""
		}
		fieldEnd := close + 2
		for fieldEnd < len(message) && ((message[fieldEnd] >= 'a' && message[fieldEnd] <= 'z') || message[fieldEnd] == '_') {
			fieldEnd++
		}
		field := message[close+2 : fieldEnd]
		if field != "remote" && field != "output" {
			return ""
		}
		return "sources[" + indexText + "]." + field
	}
	return ""
}

func validate(cfg *Config, baseDir string) (*Resolved, error) {
	apiURL, err := validateBaseURL(cfg.OpenList.APIURL, "openlist.api_url")
	if err != nil {
		return nil, err
	}
	publicPrefix, err := validateBaseURL(cfg.OpenList.PublicURL, "openlist.public_url")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.OpenList.TokenEnv) == "" {
		return nil, errors.New("openlist.token_env is required")
	}
	if !validEnvName(cfg.OpenList.TokenEnv) {
		return nil, errors.New("openlist.token_env must be a valid environment variable name")
	}
	if len(cfg.Sources) == 0 {
		return nil, errors.New("at least one source is required")
	}
	mirror := true
	if cfg.Sync.Mirror != nil {
		mirror = *cfg.Sync.Mirror
	}
	cleanupEnabled := cfg.Cleanup.Enabled != nil && *cfg.Cleanup.Enabled
	if cleanupEnabled && !mirror {
		// mirror=false means "generate/update only"; enabling cleanup would
		// contradict the operator's explicit instruction. Reject before any
		// output or database is touched.
		return nil, errors.New("cleanup.enabled: cannot be enabled while sync.mirror is false")
	}
	missingConfirmations := 3
	if cfg.Cleanup.MissingConfirmations != nil {
		if *cfg.Cleanup.MissingConfirmations < 3 {
			return nil, errors.New("cleanup.missing_confirmations must be an integer of at least 3")
		}
		missingConfirmations = *cfg.Cleanup.MissingConfirmations
	}
	if cfg.Sync.AtomicWrite != nil && !*cfg.Sync.AtomicWrite {
		return nil, errors.New("sync.atomic_write cannot be disabled")
	}
	if cfg.Sync.SkipUnchanged != nil && !*cfg.Sync.SkipUnchanged {
		return nil, errors.New("sync.skip_unchanged cannot be disabled")
	}
	if cfg.Sync.DirectoryWorkers != nil {
		if *cfg.Sync.DirectoryWorkers < 1 || *cfg.Sync.DirectoryWorkers > MaxDirectoryWorkers {
			return nil, fmt.Errorf("sync.directory_workers must be between 1 and %d", MaxDirectoryWorkers)
		}
	}
	if cfg.Sync.IntervalMinutes != nil && *cfg.Sync.IntervalMinutes < 0 {
		return nil, errors.New("sync.interval_minutes must not be negative")
	}
	workers := DefaultDirectoryWorkers
	if cfg.Sync.DirectoryWorkers != nil {
		workers = *cfg.Sync.DirectoryWorkers
	}

	resolved := &Resolved{
		APIURL:               apiURL,
		PublicPrefix:         publicPrefix,
		TokenEnv:             cfg.OpenList.TokenEnv,
		DirectoryWorkers:     workers,
		Mirror:               mirror,
		CleanupEnabled:       cleanupEnabled,
		MissingConfirmations: missingConfirmations,
		baseDir:              baseDir,
	}

	seenOutputs := map[string]int{}
	for i, src := range cfg.Sources {
		remote, err := validateRemote(src.Remote)
		if err != nil {
			return nil, fmt.Errorf("sources[%d].remote: %w", i, err)
		}
		output, err := resolveOutput(baseDir, src.Output)
		if err != nil {
			return nil, fmt.Errorf("sources[%d].output: %w", i, err)
		}
		if prev, ok := seenOutputs[output]; ok {
			return nil, fmt.Errorf("sources[%d].output duplicates sources[%d].output", i, prev)
		}
		for j := range resolved.Sources {
			overlap, err := securepath.PathsOverlap(resolved.Sources[j].Output, output)
			if err != nil {
				return nil, fmt.Errorf("sources[%d].output has an unsafe physical path", i)
			}
			if overlap {
				return nil, fmt.Errorf("sources[%d].output overlaps sources[%d].output", i, j)
			}
		}
		seenOutputs[output] = i
		resolved.Sources = append(resolved.Sources, ResolvedSource{Index: i, Remote: remote, Output: output})
	}

	dbPath, err := resolveDatabase(baseDir, cfg.Database.Path)
	if err != nil {
		return nil, err
	}
	for _, src := range resolved.Sources {
		for _, path := range databaseFiles(dbPath) {
			inside, err := securepath.DirectoryContainsPath(src.Output, path)
			if err != nil || inside || pathsOverlap(src.Output, path) {
				return nil, errors.New("database.path or SQLite sidecar must not be inside an output root")
			}
		}
	}
	resolved.DatabasePath = dbPath
	return resolved, nil
}

// validateBaseURL accepts only a clean http(s) origin plus optional path
// prefix. User info, query strings, fragments and opaque forms are rejected so
// no credential or signature can be smuggled into generated URLs.
func validateBaseURL(raw, field string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%s is required", field)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%s is not a valid URL", field)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("%s must use http or https", field)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%s must include a host", field)
	}
	if u.User != nil {
		return "", fmt.Errorf("%s must not contain user information", field)
	}
	if u.RawQuery != "" || u.ForceQuery {
		return "", fmt.Errorf("%s must not contain a query string", field)
	}
	if u.Fragment != "" {
		return "", fmt.Errorf("%s must not contain a fragment", field)
	}
	if u.Opaque != "" {
		return "", fmt.Errorf("%s is not a valid hierarchical URL", field)
	}
	prefix := u.Scheme + "://" + u.Host
	escaped := strings.TrimRight(u.EscapedPath(), "/")
	return prefix + escaped, nil
}

// validateRemote enforces a clean absolute remote path without normalization
// tricks: "." and ".." segments and empty segments are rejected, not hidden.
func validateRemote(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("must not be empty")
	}
	if len(raw) > maxRemotePathBytes {
		return "", errors.New("path exceeds the supported length limit")
	}
	if !strings.HasPrefix(raw, "/") {
		return "", errors.New("must be an absolute remote path")
	}
	if strings.ContainsAny(raw, "\x00\\") {
		return "", errors.New("must not contain NUL or backslash")
	}
	if raw == "/" {
		return "/", nil
	}
	trimmed := strings.TrimSuffix(raw, "/")
	if trimmed == "" {
		return "", errors.New("must not be empty")
	}
	for _, seg := range strings.Split(strings.TrimPrefix(trimmed, "/"), "/") {
		switch seg {
		case "":
			return "", errors.New("must not contain empty path segments")
		case ".", "..":
			return "", errors.New("must not contain '.' or '..' segments")
		}
		if hasControl(seg) {
			return "", errors.New("must not contain control characters")
		}
	}
	return trimmed, nil
}

func resolveOutput(baseDir, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("must not be empty")
	}
	abs := raw
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(baseDir, abs)
	}
	abs, err := filepath.Abs(abs)
	if err != nil {
		return "", errors.New("cannot resolve output path")
	}
	abs = filepath.Clean(abs)
	if err := securepath.ValidateDirPath(abs); err != nil {
		return "", errors.New("path contains a symlink or non-directory component")
	}
	return abs, nil
}

func resolveDatabase(baseDir, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", errors.New("database.path is required")
	}
	abs := raw
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(baseDir, abs)
	}
	abs, err := filepath.Abs(abs)
	if err != nil {
		return "", errors.New("cannot resolve database path")
	}
	abs = filepath.Clean(abs)
	if err := securepath.ValidateFilePath(abs); err != nil {
		return "", errors.New("database.path must not contain symlinks or name a non-regular file")
	}
	return abs, nil
}

// pathsOverlap reports whether a and b are equal or one is an ancestor of the
// other.
func pathsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	return isWithin(a, b) || isWithin(b, a)
}

func databaseFiles(databasePath string) []string {
	paths := []string{databasePath}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		paths = append(paths, databasePath+suffix)
	}
	return paths
}

// isWithin reports whether child is strictly inside parent.
func isWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func validEnvName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

// Token reads the configured environment variable. The value is returned to
// the caller and never persisted or logged.
func (r *Resolved) Token() (string, error) {
	value, ok := os.LookupEnv(r.TokenEnv)
	if !ok || strings.TrimSpace(value) == "" {
		return "", errors.New("configured token environment variable is not set")
	}
	return value, nil
}

// JoinRemote joins a remote directory and an entry name using forward slashes.
func JoinRemote(dir, name string) string {
	if dir == "/" {
		return "/" + name
	}
	return path.Join(dir, name)
}
