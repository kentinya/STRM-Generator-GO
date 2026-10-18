//go:build linux

// Package securepath provides descriptor-relative filesystem operations for
// paths that must remain bound to a validated directory tree. It intentionally
// supports the Linux targets used by strmgen.
package securepath

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// Dir is an open directory descriptor. Operations through Dir remain attached
// to that directory even if its configured pathname is replaced later.
type Dir struct {
	file *os.File
}

// BoundDir records an absolute directory path as an open descriptor to its
// deepest existing, verified ancestor plus any missing suffix. A missing root
// can later be created only below that descriptor. Existing roots remain bound
// to the directory inode opened here even if their pathname is replaced.
type BoundDir struct {
	mu      sync.Mutex
	path    string
	anchor  *Dir
	missing []string
	closed  bool
}

type inode struct {
	device uint64
	number uint64
}

type pathIdentity struct {
	inodes     []inode
	components []string
}

// BindDir verifies every existing component without following a symlink and
// pins the deepest existing directory. It performs no filesystem writes.
func BindDir(path string) (*BoundDir, error) {
	abs, err := absoluteClean(path)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errors.New("cannot open filesystem root")
	}
	parts := splitAbsolute(abs)
	for i, part := range parts {
		next, openErr := unix.Openat(fd, part,
			unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) {
			return &BoundDir{
				path:    abs,
				anchor:  &Dir{file: os.NewFile(uintptr(fd), abs)},
				missing: append([]string(nil), parts[i:]...),
			}, nil
		}
		if openErr != nil {
			_ = unix.Close(fd)
			return nil, safePathError("bind directory", openErr)
		}
		_ = unix.Close(fd)
		fd = next
	}
	return &BoundDir{path: abs, anchor: &Dir{file: os.NewFile(uintptr(fd), abs)}}, nil
}

// Open returns a duplicate descriptor for the bound directory. If a suffix
// was missing at bind time, create=true materializes it relative to the pinned
// ancestor and then permanently binds the resulting root inode.
func (b *BoundDir) Open(create bool, perm os.FileMode) (*Dir, error) {
	if b == nil {
		return nil, errors.New("directory binding is nil")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.anchor == nil {
		return nil, errors.New("directory binding is closed")
	}
	if len(b.missing) == 0 {
		return b.anchor.Duplicate()
	}
	if !create {
		// The path may have appeared since it was bound. Open it from the pinned
		// ancestor; O_NOFOLLOW prevents a late symlink from redirecting the root.
	}
	current, err := b.anchor.Duplicate()
	if err != nil {
		return nil, err
	}
	for i, part := range b.missing {
		next, openErr := openDirectoryAt(int(current.file.Fd()), part, create, uint32(perm.Perm()))
		_ = current.Close()
		if openErr != nil {
			return nil, safePathError("open bound directory", openErr)
		}
		current = &Dir{file: os.NewFile(uintptr(next), part)}
		if !create && i+1 < len(b.missing) {
			// A final path cannot exist if an earlier component is still missing.
			// This branch is unreachable after a successful open, but retaining it
			// makes the no-create contract explicit.
		}
	}
	if create || len(b.missing) > 0 {
		_ = b.anchor.Close()
		b.anchor = current
		b.missing = nil
		return b.anchor.Duplicate()
	}
	return current, nil
}

// VerifyPath confirms that the configured pathname still resolves to the
// same directory inode that this binding represents. Operations still use
// the descriptor, not this pathname check, so a race after verification
// cannot redirect them through a symlink.
func (b *BoundDir) VerifyPath() error {
	if b == nil {
		return errors.New("directory binding is nil")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.anchor == nil {
		return errors.New("directory binding is closed")
	}
	if len(b.missing) > 0 {
		return nil
	}
	current, err := OpenDir(b.path, false, 0)
	if err != nil {
		return errors.New("bound directory path changed")
	}
	defer current.Close()
	boundInfo, err := b.anchor.Stat()
	if err != nil {
		return errors.New("cannot inspect bound directory")
	}
	currentInfo, err := current.Stat()
	if err != nil {
		return errors.New("cannot inspect configured directory")
	}
	boundID, err := identity(boundInfo)
	if err != nil {
		return err
	}
	currentID, err := identity(currentInfo)
	if err != nil {
		return err
	}
	if boundID != currentID {
		return errors.New("bound directory path changed")
	}
	return nil
}

// Close releases the pinned descriptor.
func (b *BoundDir) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	if b.anchor != nil {
		err := b.anchor.Close()
		b.anchor = nil
		return err
	}
	return nil
}

// PathsOverlap reports whether two directory paths are equal or one is a
// physical ancestor of the other. It detects bind-mount aliases by comparing
// device/inode identities along each descriptor-relative path and rejects
// symlink/non-directory components instead of resolving through them. Missing
// suffixes are compared relative to their last existing pinned ancestor.
func PathsOverlap(a, b string) (bool, error) {
	left, err := inspectPath(a)
	if err != nil {
		return false, err
	}
	right, err := inspectPath(b)
	if err != nil {
		return false, err
	}
	for i, leftID := range left.inodes {
		for j, rightID := range right.inodes {
			if leftID != rightID {
				continue
			}
			leftSuffix := left.components[i:]
			rightSuffix := right.components[j:]
			if componentPrefix(leftSuffix, rightSuffix) || componentPrefix(rightSuffix, leftSuffix) {
				return true, nil
			}
		}
	}
	return false, nil
}

// DirectoryContainsPath reports whether target is physically inside dir. The
// target may be a file that does not yet exist; its parent directory is
// resolved without following symlinks and the final component is compared as
// a name relative to the shared physical ancestor.
func DirectoryContainsPath(dir, target string) (bool, error) {
	rootInfo, err := inspectPath(dir)
	if err != nil {
		return false, err
	}
	targetAbs, err := absoluteClean(target)
	if err != nil {
		return false, err
	}
	targetParent := filepath.Dir(targetAbs)
	targetName := filepath.Base(targetAbs)
	if err := validComponent(targetName); err != nil {
		return false, errors.New("target has an unsafe final component")
	}
	parentInfo, err := inspectPath(targetParent)
	if err != nil {
		return false, err
	}
	parentComponents := append(append([]string(nil), parentInfo.components...), targetName)
	for i, rootID := range rootInfo.inodes {
		for j, parentID := range parentInfo.inodes {
			if rootID != parentID {
				continue
			}
			rootSuffix := rootInfo.components[i:]
			parentSuffix := parentComponents[j:]
			if componentPrefix(rootSuffix, parentSuffix) {
				return true, nil
			}
		}
	}
	return false, nil
}

// OpenDir opens an absolute directory path one component at a time with
// O_NOFOLLOW. When create is true, missing components are created beneath the
// already-open parent descriptor.
func OpenDir(path string, create bool, perm os.FileMode) (*Dir, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, errors.New("cannot resolve directory path")
	}
	if strings.ContainsRune(abs, 0) {
		return nil, errors.New("invalid directory path")
	}
	abs = filepath.Clean(abs)
	if !filepath.IsAbs(abs) {
		return nil, errors.New("directory path must be absolute")
	}

	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errors.New("cannot open filesystem root")
	}
	parts := splitAbsolute(abs)
	for _, part := range parts {
		next, openErr := openDirectoryAt(fd, part, create, uint32(perm.Perm()))
		_ = unix.Close(fd)
		if openErr != nil {
			return nil, safePathError("open directory component", openErr)
		}
		fd = next
	}
	return &Dir{file: os.NewFile(uintptr(fd), abs)}, nil
}

// ValidateDirPath checks every existing path component without following
// symlinks. A missing suffix is allowed because output roots may be created by
// a later real sync; any existing non-directory component is rejected.
func ValidateDirPath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return errors.New("cannot resolve directory path")
	}
	abs = filepath.Clean(abs)
	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return errors.New("cannot open filesystem root")
	}
	defer func() { _ = unix.Close(fd) }()
	for _, part := range splitAbsolute(abs) {
		next, openErr := unix.Openat(fd, part,
			unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) {
			return nil
		}
		if openErr != nil {
			return errors.New("directory path contains an unsafe component")
		}
		_ = unix.Close(fd)
		fd = next
	}
	return nil
}

// ValidateFilePath checks an existing file path without following symlinks.
// Missing parent directories and a missing final file are allowed.
func ValidateFilePath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return errors.New("cannot resolve file path")
	}
	abs = filepath.Clean(abs)
	if err := ValidateDirPath(filepath.Dir(abs)); err != nil {
		return err
	}
	dir, err := OpenDir(filepath.Dir(abs), false, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer dir.Close()
	info, err := dir.Lstat(filepath.Base(abs))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("cannot inspect file path")
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("file path must be a regular non-symlink file")
	}
	return nil
}

// OpenSubdir opens a slash-separated relative directory under d. It rejects
// traversal and symlinks, and optionally creates missing components.
func (d *Dir) OpenSubdir(rel string, create bool, perm os.FileMode) (*Dir, error) {
	if rel == "" || rel == "." {
		return d.Duplicate()
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	current, err := d.Duplicate()
	if err != nil {
		return nil, err
	}
	for _, part := range parts {
		if err := validComponent(part); err != nil {
			current.Close()
			return nil, err
		}
		next, openErr := openDirectoryAt(int(current.file.Fd()), part, create, uint32(perm.Perm()))
		current.Close()
		if openErr != nil {
			return nil, safePathError("open directory component", openErr)
		}
		current = &Dir{file: os.NewFile(uintptr(next), part)}
	}
	return current, nil
}

// Duplicate returns another descriptor for the same directory.
func (d *Dir) Duplicate() (*Dir, error) {
	if d == nil || d.file == nil {
		return nil, errors.New("directory is closed")
	}
	fd, err := unix.Dup(int(d.file.Fd()))
	if err != nil {
		return nil, errors.New("cannot duplicate directory handle")
	}
	unix.CloseOnExec(fd)
	return &Dir{file: os.NewFile(uintptr(fd), d.file.Name())}, nil
}

// ReadDirNames returns the names of all direct children of d. It does not
// follow symlinks and is used to locate deterministic staging files without
// relying on a separately opened pathname.
func (d *Dir) ReadDirNames() ([]string, error) {
	if d == nil || d.file == nil {
		return nil, errors.New("directory is closed")
	}
	return d.file.Readdirnames(-1)
}

// Lstat reports one direct child without following a final symlink.
func (d *Dir) Lstat(name string) (os.FileInfo, error) {
	if err := validComponent(name); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(int(d.file.Fd()), name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	return f.Stat()
}

// OpenFile opens one direct child without following a final symlink.
func (d *Dir) OpenFile(name string, flags int, perm os.FileMode) (*os.File, error) {
	if err := validComponent(name); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(int(d.file.Fd()), name,
		flags|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(perm.Perm()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

// Mkdir creates a direct child directory beneath d.
func (d *Dir) Mkdir(name string, perm os.FileMode) error {
	if err := validComponent(name); err != nil {
		return err
	}
	return unix.Mkdirat(int(d.file.Fd()), name, uint32(perm.Perm()))
}

// Link links one direct child to another in the same directory. The target is
// never replaced if it already exists.
func (d *Dir) Link(oldName, newName string) error {
	if err := validComponent(oldName); err != nil {
		return err
	}
	if err := validComponent(newName); err != nil {
		return err
	}
	return unix.Linkat(int(d.file.Fd()), oldName, int(d.file.Fd()), newName, 0)
}

// Exchange atomically swaps two existing direct children of d. Both names
// must refer to entries in the same directory. It is used to publish a new
// file without ever unlinking the previous target, so a concurrent actor that
// swapped the target can be detected after the exchange and undone.
func (d *Dir) Exchange(oldName, newName string) error {
	if err := validComponent(oldName); err != nil {
		return err
	}
	if err := validComponent(newName); err != nil {
		return err
	}
	return unix.Renameat2(int(d.file.Fd()), oldName, int(d.file.Fd()), newName, unix.RENAME_EXCHANGE)
}

// Remove removes one direct child without following symlinks.
func (d *Dir) Remove(name string) error {
	if err := validComponent(name); err != nil {
		return err
	}
	return unix.Unlinkat(int(d.file.Fd()), name, 0)
}

// RemoveDir removes one empty direct child directory beneath d.
func (d *Dir) RemoveDir(name string) error {
	if err := validComponent(name); err != nil {
		return err
	}
	return unix.Unlinkat(int(d.file.Fd()), name, unix.AT_REMOVEDIR)
}

// Sync flushes directory metadata when supported by the filesystem.
func (d *Dir) Sync() error {
	if d == nil || d.file == nil {
		return errors.New("directory is closed")
	}
	return d.file.Sync()
}

// Stat reports the opened directory's current inode information.
func (d *Dir) Stat() (os.FileInfo, error) {
	if d == nil || d.file == nil {
		return nil, errors.New("directory is closed")
	}
	return d.file.Stat()
}

// FD returns the descriptor number while the Dir remains open.
func (d *Dir) FD() (int, error) {
	if d == nil || d.file == nil {
		return -1, errors.New("directory is closed")
	}
	return int(d.file.Fd()), nil
}

// ProcPath returns a stable Linux path rooted at this open directory. Callers
// must keep d open while using the returned path.
func (d *Dir) ProcPath(name string) (string, error) {
	if err := validComponent(name); err != nil {
		return "", err
	}
	if d == nil || d.file == nil {
		return "", errors.New("directory is closed")
	}
	return fmt.Sprintf("/proc/self/fd/%d/%s", d.file.Fd(), name), nil
}

// Close releases the directory descriptor.
func (d *Dir) Close() error {
	if d == nil || d.file == nil {
		return nil
	}
	err := d.file.Close()
	d.file = nil
	return err
}

func openDirectoryAt(parentFD int, name string, create bool, perm uint32) (int, error) {
	if err := validComponent(name); err != nil {
		return -1, err
	}
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	fd, err := unix.Openat(parentFD, name, flags, 0)
	if errors.Is(err, unix.ENOENT) && create {
		if mkdirErr := unix.Mkdirat(parentFD, name, perm); mkdirErr != nil && !errors.Is(mkdirErr, unix.EEXIST) {
			return -1, mkdirErr
		}
		fd, err = unix.Openat(parentFD, name, flags, 0)
	}
	return fd, err
}

func absoluteClean(path string) (string, error) {
	if strings.ContainsRune(path, 0) {
		return "", errors.New("invalid directory path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", errors.New("cannot resolve directory path")
	}
	return filepath.Clean(abs), nil
}

func inspectPath(path string) (pathIdentity, error) {
	abs, err := absoluteClean(path)
	if err != nil {
		return pathIdentity{}, err
	}
	parts := splitAbsolute(abs)
	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return pathIdentity{}, errors.New("cannot open filesystem root")
	}
	rootID, err := identityFD(fd)
	if err != nil {
		_ = unix.Close(fd)
		return pathIdentity{}, err
	}
	ids := []inode{rootID}
	currentFD := fd
	for _, part := range parts {
		next, openErr := unix.Openat(currentFD, part,
			unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if errors.Is(openErr, unix.ENOENT) {
			_ = unix.Close(currentFD)
			return pathIdentity{inodes: ids, components: parts}, nil
		}
		if openErr != nil {
			_ = unix.Close(currentFD)
			return pathIdentity{}, safePathError("inspect directory path", openErr)
		}
		_ = unix.Close(currentFD)
		currentFD = next
		id, idErr := identityFD(currentFD)
		if idErr != nil {
			_ = unix.Close(currentFD)
			return pathIdentity{}, idErr
		}
		ids = append(ids, id)
	}
	_ = unix.Close(currentFD)
	return pathIdentity{inodes: ids, components: parts}, nil
}

func identity(info os.FileInfo) (inode, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return inode{}, errors.New("cannot read directory identity")
	}
	return inode{device: uint64(stat.Dev), number: stat.Ino}, nil
}

func identityFD(fd int) (inode, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return inode{}, errors.New("cannot inspect directory identity")
	}
	return inode{device: uint64(stat.Dev), number: stat.Ino}, nil
}

func componentPrefix(prefix, value []string) bool {
	if len(prefix) > len(value) {
		return false
	}
	for i := range prefix {
		if prefix[i] != value[i] {
			return false
		}
	}
	return true
}

func splitAbsolute(path string) []string {
	rel := strings.TrimPrefix(path, string(filepath.Separator))
	if rel == "" {
		return nil
	}
	return strings.Split(rel, string(filepath.Separator))
}

func validComponent(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsRune(name, 0) || strings.Contains(name, string(filepath.Separator)) {
		return errors.New("invalid path component")
	}
	return nil
}

func safePathError(operation string, err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s: %w", operation, fs.ErrNotExist)
	}
	if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
		return fmt.Errorf("%s: unsafe component", operation)
	}
	return fmt.Errorf("%s failed", operation)
}
