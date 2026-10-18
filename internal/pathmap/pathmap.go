// Package pathmap converts remote directory entries into local relative paths
// and stable public URLs. It is pure: it performs no network or database work.
package pathmap

import (
	"errors"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"
)

// MaxSegmentBytes matches the common Linux NAME_MAX limit used by supported
// runtime targets. MaxRelativePathBytes caps path strings retained in queues,
// SQLite keys, and diagnostics.
const (
	MaxSegmentBytes      = 255
	MaxRelativePathBytes = 4096
)

// MediaExtensions is the fixed initial set of media extensions. Comparison is
// case-insensitive; the file name body keeps its original case.
var MediaExtensions = map[string]struct{}{
	".mkv":  {},
	".mp4":  {},
	".m4v":  {},
	".avi":  {},
	".mov":  {},
	".wmv":  {},
	".webm": {},
	".ts":   {},
	".m2ts": {},
	".mpg":  {},
	".mpeg": {},
}

// IsMedia reports whether name has a supported media extension.
func IsMedia(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	_, ok := MediaExtensions[ext]
	return ok
}

// LocalRelative maps a remote directory and entry name onto a local relative
// path using forward slashes, replacing the media extension with ".strm".
// The remote directory is assumed already validated (absolute, clean).
func LocalRelative(remoteDir, name string) (string, error) {
	if err := ValidateSegment(name); err != nil {
		return "", err
	}
	rel := strings.TrimPrefix(remoteDir, "/")
	stem := strings.TrimSuffix(name, path.Ext(name))
	if rel == "" {
		mapped := stem + ".strm"
		if len(mapped) > MaxRelativePathBytes {
			return "", errors.New("relative path exceeds limit")
		}
		return mapped, nil
	}
	mapped := rel + "/" + stem + ".strm"
	if len(mapped) > MaxRelativePathBytes {
		return "", errors.New("relative path exceeds limit")
	}
	return mapped, nil
}

// ValidateSegment rejects names that cannot become a safe single local path
// segment.
func ValidateSegment(name string) error {
	if name == "" || name == "." || name == ".." {
		return errors.New("invalid file name")
	}
	if !utf8.ValidString(name) {
		return errors.New("file name is not valid UTF-8")
	}
	if len(name) > MaxSegmentBytes {
		return errors.New("file name exceeds component length limit")
	}
	if strings.ContainsAny(name, "/\\") {
		return errors.New("file name must not contain a path separator")
	}
	if strings.ContainsRune(name, 0) {
		return errors.New("file name must not contain NUL")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return errors.New("file name must not contain control characters")
		}
	}
	return nil
}

// StableURL builds public_prefix + "/d/" + escaped remote path. Each remote
// segment is percent-encoded independently; directory separators are
// preserved. raw_url and signature parameters are never consulted.
func StableURL(publicPrefix, remotePath string) (string, error) {
	if publicPrefix == "" {
		return "", errors.New("public prefix is empty")
	}
	if !strings.HasPrefix(remotePath, "/") {
		return "", errors.New("remote path must be absolute")
	}
	segments := strings.Split(strings.TrimPrefix(remotePath, "/"), "/")
	escaped := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg == "" {
			return "", errors.New("remote path contains an empty segment")
		}
		if seg == "." || seg == ".." {
			return "", errors.New("remote path contains a relative segment")
		}
		escaped = append(escaped, url.PathEscape(seg))
	}
	return strings.TrimRight(publicPrefix, "/") + "/d/" + strings.Join(escaped, "/"), nil
}
