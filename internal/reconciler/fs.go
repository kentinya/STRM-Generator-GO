package reconciler

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/unix"

	"strm-generator-go/internal/securepath"
	"strm-generator-go/internal/strm"
)

// ErrTargetChanged reports that an owned target no longer holds the content
// observed before an update. The external bytes are preserved and the update
// is refused rather than overwriting them.
var ErrTargetChanged = errors.New("target changed before update")

// FileSystem is the set of local file operations the syncer needs. It is an
// interface so tests can inject failures without weakening the real
// implementation's safety checks.
type FileSystem interface {
	EnsureDir(root, relDir string) error
	ReadFile(root, relFile string) ([]byte, error)
	Lstat(root, relFile string) (os.FileInfo, error)
	Publish(root, relFile string, data []byte) error
	Update(root, relFile string, expected, data []byte) error
}

// LocalFS implements FileSystem using descriptor-relative operations. The
// configured root and every child are opened without following symlinks, so a
// pathname swap after configuration validation cannot redirect an operation
// outside the directory that was opened for that operation.
type LocalFS struct {
	roots          *localRoots
	beforeLink     func(*securepath.Dir, string) error
	beforeExchange func(*securepath.Dir, string) error
	afterUpdate    func(*securepath.Dir, string) error
	syncTemp       func(*os.File) error
	closeTemp      func(*os.File) error
}

type localRoots struct {
	mu       sync.Mutex
	bindings map[string]*securepath.BoundDir
}

// PrepareRoots binds all output paths before a remote request is made. It does
// not create missing roots; a real sync creates them later beneath the pinned
// existing ancestor, while dry-run remains side-effect free.
func (fs *LocalFS) PrepareRoots(roots []string) error {
	if fs == nil {
		return errors.New("filesystem is nil")
	}
	if fs.roots != nil {
		wanted := make(map[string]struct{}, len(roots))
		for _, root := range roots {
			abs, err := filepath.Abs(root)
			if err != nil {
				return errors.New("cannot resolve output root")
			}
			wanted[filepath.Clean(abs)] = struct{}{}
		}
		if len(wanted) != len(fs.roots.bindings) {
			return errors.New("output roots are already prepared for another round")
		}
		for root := range fs.roots.bindings {
			if _, ok := wanted[root]; !ok {
				return errors.New("output roots are already prepared for another round")
			}
		}
		return nil
	}
	prepared := &localRoots{bindings: make(map[string]*securepath.BoundDir, len(roots))}
	for _, root := range roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			closeRootBindings(prepared.bindings)
			return errors.New("cannot resolve output root")
		}
		abs = filepath.Clean(abs)
		if _, exists := prepared.bindings[abs]; exists {
			continue
		}
		bound, err := securepath.BindDir(abs)
		if err != nil {
			closeRootBindings(prepared.bindings)
			return errors.New("cannot securely bind output root")
		}
		prepared.bindings[abs] = bound
	}
	fs.roots = prepared
	return nil
}

// CloseRoots releases all descriptors retained for the current round.
func (fs *LocalFS) CloseRoots() error {
	if fs == nil || fs.roots == nil {
		return nil
	}
	roots := fs.roots
	fs.roots = nil
	return closeRootBindings(roots.bindings)
}

func closeRootBindings(bindings map[string]*securepath.BoundDir) error {
	var first error
	for _, root := range bindings {
		if err := root.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (fs LocalFS) openRoot(root string, create bool) (*securepath.Dir, error) {
	if fs.roots != nil {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, errors.New("cannot resolve output root")
		}
		abs = filepath.Clean(abs)
		fs.roots.mu.Lock()
		bound := fs.roots.bindings[abs]
		fs.roots.mu.Unlock()
		if bound == nil {
			return nil, errors.New("output root was not prepared")
		}
		if err := bound.VerifyPath(); err != nil {
			return nil, err
		}
		return bound.Open(create, 0o755)
	}
	return securepath.OpenDir(root, create, 0o755)
}

func (fs LocalFS) EnsureDir(root, relDir string) error {
	if err := validateRelativePath(relDir, true); err != nil {
		return err
	}
	opened, err := fs.openRoot(root, true)
	if err != nil {
		return err
	}
	defer opened.Close()
	if relDir == "" {
		return nil
	}
	dir, err := opened.OpenSubdir(relDir, true, 0o755)
	if err != nil {
		return err
	}
	return dir.Close()
}

func (fs LocalFS) ReadFile(root, relFile string) ([]byte, error) {
	dir, base, err := fs.openParent(root, relFile, false)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	// O_NONBLOCK ensures a target swapped to a FIFO between Lstat and Open
	// cannot hang the sync. O_NOFOLLOW is added by securepath.OpenFile.
	f, err := dir.OpenFile(base, os.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("owned target is not a regular file")
	}
	if info.Size() > strm.MaxSize {
		return nil, errors.New("owned target exceeds protocol size limit")
	}
	raw, err := io.ReadAll(io.LimitReader(f, strm.MaxSize+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > strm.MaxSize {
		return nil, errors.New("owned target exceeds protocol size limit")
	}
	return raw, nil
}

func (fs LocalFS) Lstat(root, relFile string) (os.FileInfo, error) {
	dir, base, err := fs.openParent(root, relFile, false)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return dir.Lstat(base)
}

// Publish writes to a same-directory temporary file and atomically links it
// into place. Linkat fails if another actor created the target, so this never
// overwrites an unowned file. All operations stay relative to an already-open
// parent directory.
func (fs LocalFS) Publish(root, relFile string, data []byte) error {
	if len(data) > strm.MaxSize {
		return errors.New("new STRM exceeds protocol size limit")
	}
	dir, base, err := fs.openParent(root, relFile, false)
	if err != nil {
		return err
	}
	defer dir.Close()
	if info, err := dir.Lstat(base); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("target is a symlink")
		}
		return errors.New("target already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	tmpName, tmp, err := createTempFile(dir)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Remove(tmpName) }()
	if err := writeAll(tmp, data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if fs.beforeLink != nil {
		if err := fs.beforeLink(dir, base); err != nil {
			return err
		}
	} else if err := beforePublishLink(dir, base); err != nil {
		return err
	}
	if err := dir.Link(tmpName, base); err != nil {
		return err
	}
	return dir.Sync()
}

// Update atomically replaces an already-owned STRM with new bytes. It never
// unlinks the previous target: it writes a same-directory temporary file,
// exchanges the two names, and only then removes the old content. The
// swapped-out entry must byte-for-byte match the owned content observed
// before the update; otherwise an external actor changed the target and the
// exchange is undone so their bytes are preserved.
func (fs LocalFS) Update(root, relFile string, expected, data []byte) error {
	if len(data) > strm.MaxSize {
		return errors.New("updated STRM exceeds protocol size limit")
	}
	dir, base, err := fs.openParent(root, relFile, false)
	if err != nil {
		return err
	}
	defer dir.Close()

	// Re-confirm the target is still an owned regular file immediately before
	// publishing. A symlink, special file or missing target is a conflict.
	info, err := dir.Lstat(base)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrTargetChanged
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ErrTargetChanged
	}

	tmpName, tmp, err := createTempFile(dir)
	if err != nil {
		return err
	}
	// removeTemp is cleared only if an undo failure leaves foreign bytes at
	// tmpName, which must be preserved rather than deleted.
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = dir.Remove(tmpName)
		}
	}()
	if err := writeAll(tmp, data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := fs.syncTempFile(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := fs.closeTempFile(tmp); err != nil {
		return err
	}

	if fs.beforeExchange != nil {
		if err := fs.beforeExchange(dir, base); err != nil {
			return err
		}
	} else if err := beforeUpdateExchange(dir, base); err != nil {
		return err
	}

	if err := dir.Exchange(tmpName, base); err != nil {
		return err
	}
	// The previous target now lives at tmpName. It must equal the owned content
	// read before the update; otherwise the target was replaced externally and
	// the exchange must be undone.
	if err := verifyExchanged(dir, tmpName, expected); err != nil {
		if undoErr := dir.Exchange(tmpName, base); undoErr != nil {
			// tmpName now holds the foreign bytes; do not delete them.
			removeTemp = false
			return errors.New("target changed and could not be restored")
		}
		return ErrTargetChanged
	}
	// Success: drop the old owned content and flush the directory metadata.
	if err := dir.Remove(tmpName); err != nil {
		return err
	}
	if fs.afterUpdate != nil {
		if err := fs.afterUpdate(dir, base); err != nil {
			return err
		}
	} else if err := afterUpdatePublish(dir, base); err != nil {
		return err
	}
	return dir.Sync()
}

// verifyExchanged reads a swapped-out entry without following symlinks and
// compares it to the expected owned bytes.
func verifyExchanged(dir *securepath.Dir, name string, expected []byte) error {
	f, err := dir.OpenFile(name, os.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("swapped target is not a regular file")
	}
	if info.Size() > strm.MaxSize {
		return errors.New("swapped target exceeds protocol size limit")
	}
	raw, err := io.ReadAll(io.LimitReader(f, strm.MaxSize+1))
	if err != nil {
		return err
	}
	if len(raw) > strm.MaxSize {
		return errors.New("swapped target exceeds protocol size limit")
	}
	if !bytes.Equal(raw, expected) {
		return ErrTargetChanged
	}
	return nil
}

// syncTempFile flushes a temporary file before publication. Tests can inject
// a failure here to prove the old target survives a failed durability step.
func (fs LocalFS) syncTempFile(f *os.File) error {
	if fs.syncTemp != nil {
		return fs.syncTemp(f)
	}
	return f.Sync()
}

// closeTempFile closes a temporary file before publication. Tests can inject a
// failure here to prove a close error aborts without touching the target.
func (fs LocalFS) closeTempFile(f *os.File) error {
	if fs.closeTemp != nil {
		return fs.closeTemp(f)
	}
	return f.Close()
}

func createTempFile(dir *securepath.Dir) (string, *os.File, error) {
	for attempt := 0; attempt < 10; attempt++ {
		var token [12]byte
		if _, err := rand.Read(token[:]); err != nil {
			return "", nil, errors.New("cannot generate temporary file name")
		}
		name := ".strmgen-" + hex.EncodeToString(token[:]) + ".tmp"
		f, err := dir.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			return name, f, nil
		}
		if !errors.Is(err, unix.EEXIST) {
			return "", nil, err
		}
	}
	return "", nil, errors.New("cannot allocate temporary file")
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func (fs LocalFS) openParent(root, relFile string, create bool) (*securepath.Dir, string, error) {
	if err := validateRelativePath(relFile, false); err != nil {
		return nil, "", err
	}
	parts := strings.Split(relFile, "/")
	base := parts[len(parts)-1]
	opened, err := fs.openRoot(root, create)
	if err != nil {
		return nil, "", err
	}
	if len(parts) == 1 {
		return opened, base, nil
	}
	parent, err := opened.OpenSubdir(strings.Join(parts[:len(parts)-1], "/"), create, 0o755)
	_ = opened.Close()
	if err != nil {
		return nil, "", err
	}
	return parent, base, nil
}

func validateRelativePath(rel string, allowEmpty bool) error {
	if rel == "" {
		if allowEmpty {
			return nil
		}
		return errors.New("empty relative path")
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "\\\x00") {
			return fmt.Errorf("invalid path component %q", part)
		}
	}
	return nil
}

// confine retains the pure lexical path check used by unit tests and callers
// that need a display path. LocalFS operations do not use the resulting path.
func confine(root, rel string) (string, error) {
	if err := validateRelativePath(rel, false); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", errors.New("cannot resolve output root")
	}
	abs = filepath.Clean(abs)
	target := filepath.Join(abs, filepath.FromSlash(rel))
	if !isWithin(abs, target) {
		return "", errors.New("path escapes output root")
	}
	return target, nil
}

func isWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
