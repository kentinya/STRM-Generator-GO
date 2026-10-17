package reconciler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileSystem is the set of local file operations the syncer needs. It is an
// interface so tests can inject failures without weakening the real
// implementation's safety checks.
type FileSystem interface {
	// EnsureDir creates the relative directory under root, rejecting symlinks.
	EnsureDir(root, relDir string) error
	// ReadFile reads a file inside root.
	ReadFile(root, relFile string) ([]byte, error)
	// Lstat reports a file inside root without following a final symlink.
	Lstat(root, relFile string) (os.FileInfo, error)
	// Publish atomically creates the target without overwriting anything.
	Publish(root, relFile string, data []byte) error
}

// LocalFS implements FileSystem with output-root confinement and symlink
// rejection.
type LocalFS struct{}

func (LocalFS) EnsureDir(root, relDir string) error {
	if relDir == "" {
		return os.MkdirAll(root, 0o755)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(relDir, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("invalid directory segment %q", part)
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				if err := os.Mkdir(current, 0o755); err != nil {
					return err
				}
				continue
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlinked directory component %s", part)
		}
		if !info.IsDir() {
			return fmt.Errorf("path component %s is not a directory", part)
		}
	}
	return nil
}

func (LocalFS) ReadFile(root, relFile string) ([]byte, error) {
	abs, err := confine(root, relFile)
	if err != nil {
		return nil, err
	}
	if info, err := os.Lstat(abs); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to read symlink %s", relFile)
	}
	return os.ReadFile(abs)
}

func (LocalFS) Lstat(root, relFile string) (os.FileInfo, error) {
	abs, err := confine(root, relFile)
	if err != nil {
		return nil, err
	}
	return os.Lstat(abs)
}

// Publish writes data to a same-directory temporary file, fsyncs it, then
// hard-links it into place. The link fails if the target already exists, so a
// concurrently appearing file is never overwritten.
func (LocalFS) Publish(root, relFile string, data []byte) error {
	abs, err := confine(root, relFile)
	if err != nil {
		return err
	}
	dir := filepath.Dir(abs)
	if info, err := os.Lstat(abs); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to replace symlink %s", relFile)
		}
		return fmt.Errorf("target already exists: %s", relFile)
	} else if !os.IsNotExist(err) {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".strmgen-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	if err := os.Link(tmpName, abs); err != nil {
		return err
	}
	return nil
}

// confine joins root and a forward-slash relative path, rejecting traversal
// and any existing symlinked path component so a read or publish cannot escape
// the output root.
func confine(root, rel string) (string, error) {
	if rel == "" {
		return "", errors.New("empty relative path")
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid path segment %q", part)
		}
		if strings.ContainsAny(part, "\\") {
			return "", fmt.Errorf("invalid path segment %q", part)
		}
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if !isWithin(root, abs) {
		return "", errors.New("path escapes output root")
	}
	// Reject symlinked parent components; the final component is classified by
	// the caller (Lstat for reads, Publish for writes) so a symlink target can
	// be reported as a conflict rather than a generic error.
	parts := strings.Split(rel, "/")
	current := root
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				break
			}
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("refusing symlinked path component %s", part)
		}
	}
	return abs, nil
}

// isWithin reports whether child is strictly inside parent.
func isWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
