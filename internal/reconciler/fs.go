package reconciler

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"

	"strm-generator-go/internal/securepath"
	"strm-generator-go/internal/strm"
)

// ErrTargetChanged reports that an owned target no longer holds the content
// observed before an update. The external bytes are preserved and the update
// is refused rather than overwriting them.
var ErrTargetChanged = errors.New("target changed before update")

// ErrStagingPresent reports that an unresolved update staging file exists but
// cannot be safely reconciled (for example an unreadable, foreign or
// non-regular entry, or a target whose content matches neither the desired
// bytes nor the pre-update bytes). The caller must report a conflict and must
// not delete or claim the residual.
var ErrStagingPresent = errors.New("unresolved update staging present")

// FileSystem is the set of local file operations the syncer needs. It is an
// interface so tests can inject failures without weakening the real
// implementation's safety checks.
type FileSystem interface {
	EnsureDir(root, relDir string) error
	ReadFile(root, relFile string) ([]byte, error)
	Lstat(root, relFile string) (os.FileInfo, error)
	Publish(root, relFile string, data []byte) error
	Update(root, relFile string, expected, data []byte) error
	// Recover reconciles any staging file left by an interrupted atomic update
	// before the reconciler compares the target. It only removes bytes that are
	// provably the content this run would write; any other residual is restored
	// to the target or left untouched and reported conservatively.
	Recover(root, relFile string, desired []byte, owned, dryRun bool) error
}

// LocalFS implements FileSystem using descriptor-relative operations. The
// configured root and every child are opened without following symlinks, so a
// pathname swap after configuration validation cannot redirect an operation
// outside the directory that was opened for that operation.
type LocalFS struct {
	roots          *localRoots
	beforeLink     func(*securepath.Dir, string) error
	beforeExchange func(*securepath.Dir, string) error
	afterExchange  func(*securepath.Dir, string) error
	afterUpdate    func(*securepath.Dir, string) error
	syncTemp       func(*os.File) error
	closeTemp      func(*os.File) error
	writeTemp      func(io.Writer, []byte) error
	exchange       func(*securepath.Dir, string, string) error
	remove         func(*securepath.Dir, string) error
	syncDir        func(*securepath.Dir) error
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
// unlinks the previous target: it writes a same-directory deterministic
// staging file, binds it to the exact object this call wrote, exchanges the
// two names, and only then removes the displaced entry. Both sides of the
// exchange are proven by device/inode identity after the swap: base must hold
// the object this call wrote and staging must hold the previously owned
// object. A foreign replacement at either name is detected and preserved
// rather than published or deleted. The deterministic staging name encodes
// the previous content hash so crash recovery can distinguish the displaced
// old content from foreign bytes.
func (fs LocalFS) Update(root, relFile string, expected, data []byte) error {
	if len(data) > strm.MaxSize {
		return errors.New("updated STRM exceeds protocol size limit")
	}
	dir, base, err := fs.openParent(root, relFile, false)
	if err != nil {
		return err
	}
	defer dir.Close()

	// Re-confirm the target is still an owned regular file holding exactly the
	// bytes observed before the update. A symlink, special file, missing target
	// or changed content is a conflict.
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
	oldID, haveOld := identityOf(info)
	baseRaw, err := readRegularAt(dir, base)
	if err != nil || !bytes.Equal(baseRaw, expected) {
		return ErrTargetChanged
	}

	staging := newStagingName(base, expected)
	if _, err := dir.Lstat(staging); err == nil {
		// A staging file from an earlier interrupted update is still present.
		// Never overwrite unknown content; recovery must run first.
		return ErrStagingPresent
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	tmp, err := dir.OpenFile(staging, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	tmpInfo, err := tmp.Stat()
	if err != nil {
		_ = tmp.Close()
		// The staging object was just created, but without a trustworthy
		// identity it must be left rather than removed by name.
		return err
	}
	tmpID, haveID := identityOf(tmpInfo)

	if err := fs.writeTempFile(tmp, data); err != nil {
		_ = tmp.Close()
		fs.removeIfIdentity(dir, staging, tmpID, haveID)
		return err
	}
	if err := fs.syncTempFile(tmp); err != nil {
		_ = tmp.Close()
		fs.removeIfIdentity(dir, staging, tmpID, haveID)
		return err
	}
	if err := fs.closeTempFile(tmp); err != nil {
		fs.removeIfIdentity(dir, staging, tmpID, haveID)
		return err
	}

	// Bind the object written above to the staging name before publishing. A
	// foreign object parked at the deterministic name must never be exchanged
	// into the target or removed by the failure cleanup.
	if !entryMatches(dir, staging, tmpID, haveID, data) {
		return ErrStagingPresent
	}

	if fs.beforeExchange != nil {
		if err := fs.beforeExchange(dir, base); err != nil {
			fs.removeIfMatches(dir, staging, tmpID, haveID, data)
			return err
		}
	} else if err := beforeUpdateExchange(dir, base); err != nil {
		fs.removeIfMatches(dir, staging, tmpID, haveID, data)
		return err
	}
	// The pre-exchange hook may pause while an external writer replaces the
	// staging object; re-confirm it is still ours before the swap.
	if !entryMatches(dir, staging, tmpID, haveID, data) {
		return ErrStagingPresent
	}

	if err := fs.exchangeNames(dir, staging, base); err != nil {
		fs.removeIfMatches(dir, staging, tmpID, haveID, data)
		return err
	}

	if fs.afterExchange != nil {
		if err := fs.afterExchange(dir, base); err != nil {
			return err
		}
	} else if err := afterUpdateExchange(dir, base); err != nil {
		return err
	}

	// base must hold exactly the object this call published; the content check
	// guards against a reused inode number after an external replacement.
	if !entryMatches(dir, base, tmpID, haveID, data) {
		fs.undoExchange(dir, staging, base, tmpID, haveID, data)
		return ErrTargetChanged
	}
	// staging must hold exactly the previously owned object.
	if !entryMatches(dir, staging, oldID, haveOld, expected) {
		fs.undoExchange(dir, staging, base, tmpID, haveID, data)
		return ErrTargetChanged
	}

	// Proven: base is the new object and staging is the displaced old owned
	// object. Drop the old content and flush the directory metadata.
	if err := fs.removeFile(dir, staging); err != nil {
		return err
	}
	if fs.afterUpdate != nil {
		if err := fs.afterUpdate(dir, base); err != nil {
			return err
		}
	} else if err := afterUpdatePublish(dir, base); err != nil {
		return err
	}
	return fs.syncDirectory(dir)
}

// undoExchange restores a displaced entry to the target after a failed
// verification. It only acts when the target still holds exactly the object
// this call published (device/inode identity plus content); otherwise it
// leaves the target and every residual untouched. The new object at staging is
// removed only with the same identity-and-content proof, so a foreign
// replacement anywhere in the window can never be destroyed, even when the
// filesystem reuses an unlinked inode number.
func (fs LocalFS) undoExchange(dir *securepath.Dir, staging, base string, newID fileIdentity, haveNew bool, data []byte) {
	if !entryMatches(dir, base, newID, haveNew, data) {
		// The target is no longer the object we published; never move it.
		return
	}
	if err := fs.exchangeNames(dir, staging, base); err != nil {
		return
	}
	// staging now holds the object we published; remove it only if proven.
	fs.removeIfMatches(dir, staging, newID, haveNew, data)
}

// Recover reconciles a deterministic staging file left by an interrupted
// atomic update. It is proof-based: the staging name encodes the hash of the
// previous owned content, so a residual that byte-equals that hash is
// provably the displaced old content, while any other residual may hold
// foreign bytes. When the exchange already completed it restores the displaced
// content to the target and removes only this run's own desired output; if the
// displaced content cannot be proven to be the old owned content the caller is
// told to treat the target as an unowned conflict instead of silently
// updating it. When dryRun is true it only detects the residual.
func (fs LocalFS) Recover(root, relFile string, desired []byte, owned, dryRun bool) error {
	dir, base, err := fs.openParent(root, relFile, false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer dir.Close()

	stagingNames, err := findStagingFiles(dir, base)
	if err != nil {
		return err
	}
	if len(stagingNames) == 0 {
		return nil
	}
	if dryRun {
		return ErrStagingPresent
	}
	// More than one residual for the same target is ambiguous: never guess
	// which object may be reclaimed.
	if len(stagingNames) != 1 {
		return ErrStagingPresent
	}
	staging := stagingNames[0]
	stInfo, err := dir.Lstat(staging)
	if err != nil {
		return err
	}
	if stInfo.Mode()&os.ModeSymlink != 0 || !stInfo.Mode().IsRegular() {
		return ErrStagingPresent
	}
	if !owned {
		// A staging file with no trustworthy ownership must never be deleted or
		// reclaimed.
		return ErrStagingPresent
	}

	stagingRaw, err := readRegularAt(dir, staging)
	if err != nil {
		return ErrStagingPresent
	}
	oldHash, ok := stagingOldHash(staging, base)
	provenOld := ok && hashPrefix(stagingRaw, len(oldHash)) == oldHash

	targetInfo, terr := dir.Lstat(base)
	targetExists := terr == nil
	if terr != nil && !errors.Is(terr, os.ErrNotExist) {
		return terr
	}
	if !targetExists {
		if bytes.Equal(stagingRaw, desired) {
			// The new content was never published; drop our own staging output and
			// let the normal creation path run.
			if err := fs.removeFile(dir, staging); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		}
		return ErrStagingPresent
	}
	if targetInfo.Mode()&os.ModeSymlink != 0 || !targetInfo.Mode().IsRegular() {
		return ErrStagingPresent
	}
	targetRaw, err := readRegularAt(dir, base)
	if err != nil {
		return ErrStagingPresent
	}

	switch {
	case bytes.Equal(targetRaw, desired):
		// The exchange completed. Put the displaced entry back on the target and
		// remove only the new copy, which is provably byte-equal to desired. If
		// the displaced content cannot be proven to be the previous owned
		// content, report a conservative conflict instead of letting the normal
		// comparison overwrite a possibly foreign STRM.
		if err := fs.exchangeNames(dir, staging, base); err != nil {
			return ErrStagingPresent
		}
		info, err := dir.Lstat(staging)
		if err != nil {
			return ErrStagingPresent
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return ErrStagingPresent
		}
		raw, err := readRegularAt(dir, staging)
		if err != nil || !bytes.Equal(raw, desired) {
			return ErrStagingPresent
		}
		if err := fs.removeFile(dir, staging); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if provenOld {
			return nil
		}
		return ErrStagingPresent
	case bytes.Equal(stagingRaw, desired):
		// The staged new content was never exchanged; drop only our own output.
		if err := fs.removeFile(dir, staging); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	default:
		// Neither side matches what this run would write. Leave both untouched
		// and report a conservative conflict.
		return ErrStagingPresent
	}
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

// writeTempFile writes the complete new document to the staging file. Tests
// can inject a failure here to prove a partial write never reaches the target.
func (fs LocalFS) writeTempFile(w io.Writer, data []byte) error {
	if fs.writeTemp != nil {
		return fs.writeTemp(w, data)
	}
	return writeAll(w, data)
}

// exchangeNames atomically swaps two names in the same directory. Tests can
// inject a failure in either the publish or the rollback direction.
func (fs LocalFS) exchangeNames(dir *securepath.Dir, a, b string) error {
	if fs.exchange != nil {
		return fs.exchange(dir, a, b)
	}
	return dir.Exchange(a, b)
}

// removeFile removes one owned direct child. Tests can inject a failure to
// exercise the durability boundary after publication.
func (fs LocalFS) removeFile(dir *securepath.Dir, name string) error {
	if fs.remove != nil {
		return fs.remove(dir, name)
	}
	return dir.Remove(name)
}

// syncDirectory flushes directory metadata after a publish. Tests can inject a
// failure to distinguish "published" from "durably recorded".
func (fs LocalFS) syncDirectory(dir *securepath.Dir) error {
	if fs.syncDir != nil {
		return fs.syncDir(dir)
	}
	return dir.Sync()
}

// updateStagingSuffix terminates every deterministic update staging name.
const updateStagingSuffix = ".update.tmp"

// newStagingName returns the deterministic, hidden staging name used for
// atomic replacement of one target. It encodes a hash of the previous owned
// content so crash recovery can prove whether a residual is the displaced old
// content or possibly foreign bytes. Determinism lets the next run reconcile
// an interruption instead of guessing at a random name.
func newStagingName(base string, expected []byte) string {
	return stagingPrefix(base) + hashPrefix(expected, 16) + updateStagingSuffix
}

func stagingPrefix(base string) string {
	return ".strmgen-" + hashPrefix([]byte(base), 16) + "-"
}

// stagingOldHash extracts the encoded previous-content hash from a staging
// name belonging to base.
func stagingOldHash(name, base string) (string, bool) {
	prefix := stagingPrefix(base)
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, updateStagingSuffix) {
		return "", false
	}
	mid := name[len(prefix) : len(name)-len(updateStagingSuffix)]
	if len(mid) != 16 {
		return "", false
	}
	return mid, true
}

// findStagingFiles lists every deterministic staging entry for base. It reads
// the directory through the already-open descriptor so it cannot be redirected
// by a pathname swap.
func findStagingFiles(dir *securepath.Dir, base string) ([]string, error) {
	names, err := dir.ReadDirNames()
	if err != nil {
		return nil, err
	}
	prefix := stagingPrefix(base)
	var found []string
	for _, name := range names {
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, updateStagingSuffix) {
			found = append(found, name)
		}
	}
	return found, nil
}

// hashPrefix returns the first n hex characters of the SHA-256 digest of data.
func hashPrefix(data []byte, n int) string {
	sum := sha256.Sum256(data)
	encoded := hex.EncodeToString(sum[:])
	if n > len(encoded) {
		n = len(encoded)
	}
	return encoded[:n]
}

// entryIdentity reports whether the direct child name currently refers to the
// exact object identified by want.
func entryIdentity(dir *securepath.Dir, name string, want fileIdentity, have bool) bool {
	if !have {
		return false
	}
	id, ok := entryIdentityOf(dir, name)
	return ok && id == want
}

// entryMatches reports whether the direct child name currently refers to the
// exact object identified by want and still holds the expected content. The
// content check defends against inode-number reuse after a foreign
// replacement.
func entryMatches(dir *securepath.Dir, name string, want fileIdentity, have bool, content []byte) bool {
	if !entryIdentity(dir, name, want, have) {
		return false
	}
	raw, err := readRegularAt(dir, name)
	if err != nil {
		return false
	}
	return bytes.Equal(raw, content)
}

// entryIdentityOf reports the device/inode identity of a direct child.
func entryIdentityOf(dir *securepath.Dir, name string) (fileIdentity, bool) {
	info, err := dir.Lstat(name)
	if err != nil {
		return fileIdentity{}, false
	}
	return identityOf(info)
}

// removeIfIdentity removes name only when it still refers to the exact object
// the caller created. Tests can inject a failure through fs.remove.
func (fs LocalFS) removeIfIdentity(dir *securepath.Dir, name string, want fileIdentity, have bool) {
	if !entryIdentity(dir, name, want, have) {
		return
	}
	_ = fs.removeFile(dir, name)
}

// removeIfMatches removes name only when it still refers to the exact object
// the caller created and still holds the expected content.
func (fs LocalFS) removeIfMatches(dir *securepath.Dir, name string, want fileIdentity, have bool, content []byte) {
	if !entryMatches(dir, name, want, have, content) {
		return
	}
	_ = fs.removeFile(dir, name)
}

// fileIdentity is a device/inode pair used to prove that a name still refers
// to the exact object this process created.
type fileIdentity struct {
	device uint64
	number uint64
}

func identityOf(info os.FileInfo) (fileIdentity, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileIdentity{}, false
	}
	return fileIdentity{device: uint64(stat.Dev), number: stat.Ino}, true
}

// readRegularAt reads one bounded, no-follow regular file beneath dir.
func readRegularAt(dir *securepath.Dir, name string) ([]byte, error) {
	f, err := dir.OpenFile(name, os.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("entry is not a regular file")
	}
	if info.Size() > strm.MaxSize {
		return nil, errors.New("entry exceeds protocol size limit")
	}
	raw, err := io.ReadAll(io.LimitReader(f, strm.MaxSize+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > strm.MaxSize {
		return nil, errors.New("entry exceeds protocol size limit")
	}
	return raw, nil
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
