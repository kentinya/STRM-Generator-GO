// Package config parses and validates the strmgen YAML contract before any
// output is produced. It deliberately keeps the set of supported fields small
// and rejects unknown fields so a typo cannot silently disable a safety
// invariant.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultDirectoryWorkers is used when sync.directory_workers is omitted.
const DefaultDirectoryWorkers = 4

// MaxDirectoryWorkers bounds the per-round directory concurrency.
const MaxDirectoryWorkers = 32

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
	APIURL           string
	PublicPrefix     string
	TokenEnv         string
	DirectoryWorkers int
	DatabasePath     string
	Sources          []ResolvedSource
	baseDir          string
}

// Load reads, strictly decodes and validates a YAML file.
func Load(file string) (*Resolved, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("config file is empty")
	}
	cfg := &Config{}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	// Reject a second YAML document: the contract is a single document.
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return nil, errors.New("config must contain exactly one YAML document")
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}
	return validate(cfg, filepath.Dir(abs))
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
	if strings.ContainsAny(cfg.OpenList.TokenEnv, "= \t") {
		return nil, errors.New("openlist.token_env must be an environment variable name")
	}
	if len(cfg.Sources) == 0 {
		return nil, errors.New("at least one source is required")
	}
	if cfg.Cleanup.Enabled != nil && *cfg.Cleanup.Enabled {
		return nil, errors.New("cleanup.enabled is not supported by this task; set cleanup.enabled: false")
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
		APIURL:           apiURL,
		PublicPrefix:     publicPrefix,
		TokenEnv:         cfg.OpenList.TokenEnv,
		DirectoryWorkers: workers,
		baseDir:          baseDir,
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
			return nil, fmt.Errorf("sources[%d].output duplicates sources[%d].output (%s)", i, prev, output)
		}
		for j := range resolved.Sources {
			if pathsOverlap(resolved.Sources[j].Output, output) {
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
		if isWithin(src.Output, dbPath) || dbPath == src.Output {
			return nil, fmt.Errorf("database.path must not be inside an output root (%s)", dbPath)
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
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("must not be empty")
	}
	abs := raw
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(baseDir, abs)
	}
	abs, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if err := rejectSymlink(abs); err != nil {
		return "", err
	}
	return abs, nil
}

func resolveDatabase(baseDir, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("database.path is required")
	}
	abs := raw
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(baseDir, abs)
	}
	abs, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	if err := rejectSymlink(abs); err != nil {
		return "", err
	}
	return abs, nil
}

// rejectSymlink refuses an existing symlink at the configured path so a root
// cannot silently point outside the intended tree.
func rejectSymlink(p string) error {
	info, err := os.Lstat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink at %s", p)
	}
	return nil
}

// pathsOverlap reports whether a and b are equal or one is an ancestor of the
// other.
func pathsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	return isWithin(a, b) || isWithin(b, a)
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

// Token reads the configured environment variable. The value is returned to
// the caller and never persisted or logged.
func (r *Resolved) Token() (string, error) {
	value, ok := os.LookupEnv(r.TokenEnv)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("environment variable %s is not set", r.TokenEnv)
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
