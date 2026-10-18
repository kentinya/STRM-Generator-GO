package reconciler

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"

	"strm-generator-go/internal/securepath"
	"strm-generator-go/internal/strm"
)

func TestConfineRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"../escape", "a/../../escape", "", "a/./b", "a\\b"} {
		if _, err := confine(root, rel); err == nil {
			t.Fatalf("confine(%q) must fail", rel)
		}
	}
	got, err := confine(root, "sub/deep/A.strm")
	if err != nil {
		t.Fatalf("confine: %v", err)
	}
	if !isWithin(root, got) {
		t.Fatalf("result escaped root: %s", got)
	}
}

func TestPublishDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	fs := LocalFS{}
	if err := fs.EnsureDir(root, "sub"); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	if err := fs.Publish(root, "sub/A.strm", []byte("first")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := fs.Publish(root, "sub/A.strm", []byte("second")); err == nil {
		t.Fatal("publish must refuse to overwrite")
	}
	raw, err := os.ReadFile(filepath.Join(root, "sub", "A.strm"))
	if err != nil || string(raw) != "first" {
		t.Fatalf("original content changed: %q %v", raw, err)
	}
}

func TestPublishDoesNotOverwriteTargetAppearingAfterInitialCheck(t *testing.T) {
	root := t.TempDir()
	fs := LocalFS{beforeLink: func(dir *securepath.Dir, target string) error {
		if target != "A.strm" {
			t.Fatalf("unexpected target %q", target)
		}
		f, err := dir.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if _, err := f.Write([]byte("external")); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	}}
	if err := fs.Publish(root, "A.strm", []byte("generator")); err == nil {
		t.Fatal("a target appearing after the initial check must fail publication")
	}
	raw, err := os.ReadFile(filepath.Join(root, "A.strm"))
	if err != nil || string(raw) != "external" {
		t.Fatalf("racing external target was modified: %q %v", raw, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "A.strm" {
		t.Fatalf("temporary file was not removed after link refusal: %v", entries)
	}
}

func TestPublishRefusesSymlinkTarget(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "sentinel")
	if err := os.WriteFile(target, []byte("sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "A.strm")); err != nil {
		t.Fatal(err)
	}
	fs := LocalFS{}
	if err := fs.Publish(root, "A.strm", []byte("x")); err == nil {
		t.Fatal("publish over a symlink must fail")
	}
	raw, _ := os.ReadFile(target)
	if string(raw) != "sentinel" {
		t.Fatalf("sentinel was modified: %q", raw)
	}
}

func TestUpdateAtomicallyReplacesAndRemovesOldContent(t *testing.T) {
	root := t.TempDir()
	fs := LocalFS{}
	if err := fs.EnsureDir(root, "sub"); err != nil {
		t.Fatal(err)
	}
	old := []byte("# strm-probe-version:1\nhttps://e.invalid/d/A.mkv\n")
	newer := []byte("# strm-probe-version:1\nhttps://e.invalid/d/A.mkv\n# strm-probe-size:5\n")
	if err := fs.Publish(root, "sub/A.strm", old); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := fs.Update(root, "sub/A.strm", old, newer); err != nil {
		t.Fatalf("update: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "sub", "A.strm"))
	if string(raw) != string(newer) {
		t.Fatalf("target not replaced: %q", raw)
	}
	entries, err := os.ReadDir(filepath.Join(root, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "A.strm" {
		t.Fatalf("old content or temporary file was not removed: %v", entries)
	}
}

func TestUpdatePreservesExternallyReplacedTarget(t *testing.T) {
	root := t.TempDir()
	fs := LocalFS{beforeExchange: func(dir *securepath.Dir, target string) error {
		if err := dir.Remove(target); err != nil {
			return err
		}
		f, err := dir.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if _, err := f.Write([]byte("external")); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	}}
	old := []byte("# strm-probe-version:1\nhttps://e.invalid/d/A.mkv\n")
	if err := fs.Publish(root, "A.strm", old); err != nil {
		t.Fatal(err)
	}
	if err := fs.Update(root, "A.strm", old, []byte("replacement")); err == nil {
		t.Fatal("external replacement must fail the update")
	}
	raw, _ := os.ReadFile(filepath.Join(root, "A.strm"))
	if string(raw) != "external" {
		t.Fatalf("external bytes were not preserved: %q", raw)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "A.strm" {
		t.Fatalf("temporary file was not cleaned up: %v", entries)
	}
}

func TestUpdateRefusesSymlinkAndSpecialTargets(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "sentinel")
	if err := os.WriteFile(outside, []byte("sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.strm")); err != nil {
		t.Fatal(err)
	}
	if err := (LocalFS{}).Update(root, "link.strm", nil, []byte("x")); err == nil {
		t.Fatal("update over a symlink must fail")
	}
	if raw, _ := os.ReadFile(outside); string(raw) != "sentinel" {
		t.Fatalf("symlink target changed: %q", raw)
	}
	if err := unix.Mkfifo(filepath.Join(root, "pipe.strm"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (LocalFS{}).Update(root, "pipe.strm", nil, []byte("x")); err == nil {
		t.Fatal("update over a FIFO must fail")
	}
}

func TestUpdateRejectsOversizedDocument(t *testing.T) {
	root := t.TempDir()
	if err := (LocalFS{}).Publish(root, "A.strm", []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := (LocalFS{}).Update(root, "A.strm", []byte("old"), make([]byte, strm.MaxSize+1)); err == nil {
		t.Fatal("oversized update must be rejected before writing")
	}
	raw, _ := os.ReadFile(filepath.Join(root, "A.strm"))
	if string(raw) != "old" {
		t.Fatalf("rejected update changed the target: %q", raw)
	}
}

func TestUpdateFailureStagesPreserveTargetAndCleanTemp(t *testing.T) {
	cases := map[string]func() *LocalFS{
		"sync": func() *LocalFS {
			return &LocalFS{syncTemp: func(*os.File) error { return errors.New("injected sync failure") }}
		},
		"close": func() *LocalFS {
			return &LocalFS{closeTemp: func(f *os.File) error { _ = f.Close(); return errors.New("injected close failure") }}
		},
		"exchange": func() *LocalFS {
			return &LocalFS{beforeExchange: func(*securepath.Dir, string) error { return errors.New("injected exchange failure") }}
		},
	}
	for name, factory := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			old := []byte("# strm-probe-version:1\nhttps://e.invalid/d/A.mkv\n")
			if err := (LocalFS{}).Publish(root, "A.strm", old); err != nil {
				t.Fatal(err)
			}
			if err := factory().Update(root, "A.strm", old, []byte("replacement")); err == nil {
				t.Fatalf("%s failure must abort the update", name)
			}
			raw, _ := os.ReadFile(filepath.Join(root, "A.strm"))
			if string(raw) != string(old) {
				t.Fatalf("%s failure changed the target: %q", name, raw)
			}
			entries, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != "A.strm" {
				t.Fatalf("%s failure left a temporary file: %v", name, entries)
			}
		})
	}
}

func TestEnsureDirRefusesSymlinkedComponent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	fs := LocalFS{}
	if err := fs.EnsureDir(root, "link/sub"); err == nil {
		t.Fatal("symlinked component must be refused")
	}
}

func TestReadFileRejectsFIFOAndOversizedTargets(t *testing.T) {
	root := t.TempDir()
	if err := unix.Mkfifo(filepath.Join(root, "pipe.strm"), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := LocalFS{}
	if _, err := fs.ReadFile(root, "pipe.strm"); err == nil {
		t.Fatal("FIFO must not be read as an owned STRM")
	}
	if err := os.WriteFile(filepath.Join(root, "large.strm"), make([]byte, 65537), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFile(root, "large.strm"); err == nil {
		t.Fatal("oversized file must be rejected before parsing")
	}
}

func TestReadFileRejectsSymlinkAndDirectoryTargets(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "sentinel")
	if err := os.WriteFile(outside, []byte("sentinel"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.strm")); err != nil {
		t.Fatal(err)
	}
	if _, err := (LocalFS{}).ReadFile(root, "link.strm"); err == nil {
		t.Fatal("symlink target must be rejected")
	}
	if err := os.Mkdir(filepath.Join(root, "dir.strm"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := (LocalFS{}).ReadFile(root, "dir.strm"); err == nil {
		t.Fatal("directory target must be rejected")
	}
	if raw, err := os.ReadFile(outside); err != nil || string(raw) != "sentinel" {
		t.Fatalf("symlink target changed: %q %v", raw, err)
	}
}

func TestLockIsExclusive(t *testing.T) {
	dir := t.TempDir()
	first, err := acquireLockAt(dir)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, err := acquireLockAt(dir); !errors.Is(err, ErrBusy) {
		t.Fatalf("second acquire = %v, want ErrBusy", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	again, err := acquireLockAt(dir)
	if err != nil {
		t.Fatalf("re-acquire: %v", err)
	}
	again.Release()
}

func TestLockIsCrossProcessSafe(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockHelperProcess$")
	cmd.Env = append(os.Environ(), "STRMGEN_TEST_LOCK_HELPER=1", "STRMGEN_TEST_LOCK_DIR="+dir)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		t.Fatalf("child did not report lock acquisition: %v %s", err, stderr.String())
	}
	if strings.TrimSpace(line) != "LOCKED" {
		_ = stdin.Close()
		_ = cmd.Wait()
		t.Fatalf("unexpected child output: %q %s", line, stderr.String())
	}
	if _, err := acquireLockAt(dir); !errors.Is(err, ErrBusy) {
		t.Fatalf("second acquire = %v, want ErrBusy", err)
	}
	_ = stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("child exit: %v %s", err, stderr.String())
	}
	lock, err := acquireLockAt(dir)
	if err != nil {
		t.Fatalf("lock was not released after process exit: %v", err)
	}
	_ = lock.Release()
}

func TestLockHelperProcess(t *testing.T) {
	if os.Getenv("STRMGEN_TEST_LOCK_HELPER") != "1" {
		return
	}
	lock, err := acquireLockAt(os.Getenv("STRMGEN_TEST_LOCK_DIR"))
	if err != nil {
		_, _ = os.Stderr.WriteString("lock helper could not acquire lock\n")
		os.Exit(2)
	}
	_, _ = os.Stdout.WriteString("LOCKED\n")
	_, _ = io.Copy(io.Discard, os.Stdin)
	_ = lock.Release()
	os.Exit(0)
}

func writeUpdateFixture(t *testing.T, root string) (old, newer []byte) {
	t.Helper()
	old = []byte("# strm-probe-version:1\nhttps://e.invalid/d/A.mkv\n")
	newer = []byte("# strm-probe-version:1\n# strm-probe-size:5\nhttps://e.invalid/d/A.mkv\n")
	if err := (LocalFS{}).Publish(root, "A.strm", old); err != nil {
		t.Fatalf("publish fixture: %v", err)
	}
	return old, newer
}

func replaceTargetBytes(dir *securepath.Dir, target, content string) error {
	if err := dir.Remove(target); err != nil {
		return err
	}
	f, err := dir.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write([]byte(content)); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func onlyEntryNamed(t *testing.T, root, name string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != name {
		t.Fatalf("unexpected directory entries: %v", entries)
	}
}

func residualBytes(t *testing.T, root, target string) []byte {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == target {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	return nil
}

// F-10: a second external replacement between verification and rollback must
// never be deleted by the rollback or by deferred cleanup.
func TestUpdateSecondReplacementBeforeRollbackPreservesBoth(t *testing.T) {
	root := t.TempDir()
	old, newer := writeUpdateFixture(t, root)
	fs := LocalFS{
		beforeExchange: func(dir *securepath.Dir, target string) error {
			return replaceTargetBytes(dir, target, "external-A")
		},
		afterExchange: func(dir *securepath.Dir, target string) error {
			return replaceTargetBytes(dir, target, "external-B")
		},
	}
	if err := fs.Update(root, "A.strm", old, newer); !errors.Is(err, ErrTargetChanged) {
		t.Fatalf("update = %v, want ErrTargetChanged", err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "A.strm"))
	if string(raw) != "external-B" {
		t.Fatalf("second external target was not preserved: %q", raw)
	}
	if got := residualBytes(t, root, "A.strm"); string(got) != "external-A" {
		t.Fatalf("first external target was not preserved: %q", got)
	}
}

// F-10/F-12: when the rollback exchange itself fails, both the published
// object and the foreign residual must survive and no cleanup may delete them.
func TestUpdateRollbackFailurePreservesForeignResidual(t *testing.T) {
	root := t.TempDir()
	old, newer := writeUpdateFixture(t, root)
	calls := 0
	fs := LocalFS{
		beforeExchange: func(dir *securepath.Dir, target string) error {
			return replaceTargetBytes(dir, target, "external")
		},
		exchange: func(dir *securepath.Dir, a, b string) error {
			calls++
			if calls == 1 {
				return dir.Exchange(a, b)
			}
			return errors.New("injected rollback failure")
		},
	}
	if err := fs.Update(root, "A.strm", old, newer); !errors.Is(err, ErrTargetChanged) {
		t.Fatalf("update = %v, want ErrTargetChanged", err)
	}
	if got := residualBytes(t, root, "A.strm"); string(got) != "external" {
		t.Fatalf("foreign residual was lost: %q", got)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "A.strm")); string(raw) != string(newer) {
		t.Fatalf("published object was changed by failed rollback: %q", raw)
	}
}

// F-13: an external writer replacing the staging object just before the
// exchange must never be published as the target, and its bytes must survive.
func TestUpdateForeignStagingReplacementIsNotPublished(t *testing.T) {
	root := t.TempDir()
	old, newer := writeUpdateFixture(t, root)
	staging := newStagingName("A.strm", old, newer)
	fs := LocalFS{beforeExchange: func(dir *securepath.Dir, _ string) error {
		return replaceTargetBytes(dir, staging, "foreign-staging-object")
	}}
	if err := fs.Update(root, "A.strm", old, newer); !errors.Is(err, ErrStagingPresent) {
		t.Fatalf("update = %v, want ErrStagingPresent", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "A.strm")); string(raw) != string(old) {
		t.Fatalf("foreign staging object was published: %q", raw)
	}
	if got := residualBytes(t, root, "A.strm"); string(got) != "foreign-staging-object" {
		t.Fatalf("foreign staging object was deleted: %q", got)
	}
}

// F-13: when the target vanishes and the staging object is replaced by an
// external writer, the failure path must not delete the foreign object.
func TestUpdateForeignStagingSurvivesMissingTarget(t *testing.T) {
	root := t.TempDir()
	old, newer := writeUpdateFixture(t, root)
	staging := newStagingName("A.strm", old, newer)
	fs := LocalFS{beforeExchange: func(dir *securepath.Dir, target string) error {
		if err := replaceTargetBytes(dir, staging, "foreign-staging-object"); err != nil {
			return err
		}
		return dir.Remove(target)
	}}
	if err := fs.Update(root, "A.strm", old, newer); !errors.Is(err, ErrStagingPresent) {
		t.Fatalf("update = %v, want ErrStagingPresent", err)
	}
	if _, err := os.Stat(filepath.Join(root, "A.strm")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed target unexpectedly present: %v", err)
	}
	if got := residualBytes(t, root, "A.strm"); string(got) != "foreign-staging-object" {
		t.Fatalf("foreign staging object was deleted: %q", got)
	}
}

func TestUpdatePublishExchangeFailurePreservesOldTarget(t *testing.T) {
	root := t.TempDir()
	old, newer := writeUpdateFixture(t, root)
	fs := LocalFS{exchange: func(*securepath.Dir, string, string) error {
		return errors.New("injected publish failure")
	}}
	if err := fs.Update(root, "A.strm", old, newer); err == nil {
		t.Fatal("publish exchange failure must abort")
	}
	raw, _ := os.ReadFile(filepath.Join(root, "A.strm"))
	if string(raw) != string(old) {
		t.Fatalf("old target changed: %q", raw)
	}
	onlyEntryNamed(t, root, "A.strm")
}

func TestUpdateTempWriteFailurePreservesOldTarget(t *testing.T) {
	root := t.TempDir()
	old, newer := writeUpdateFixture(t, root)
	fs := LocalFS{writeTemp: func(io.Writer, []byte) error { return errors.New("injected write failure") }}
	if err := fs.Update(root, "A.strm", old, newer); err == nil {
		t.Fatal("temporary write failure must abort")
	}
	raw, _ := os.ReadFile(filepath.Join(root, "A.strm"))
	if string(raw) != string(old) {
		t.Fatalf("old target changed: %q", raw)
	}
	onlyEntryNamed(t, root, "A.strm")
}

func TestUpdateDirectorySyncFailureKeepsNewTargetWithoutStaging(t *testing.T) {
	root := t.TempDir()
	old, newer := writeUpdateFixture(t, root)
	fs := LocalFS{syncDir: func(*securepath.Dir) error { return errors.New("injected dir sync failure") }}
	if err := fs.Update(root, "A.strm", old, newer); err == nil {
		t.Fatal("directory sync failure must surface as an error")
	}
	raw, _ := os.ReadFile(filepath.Join(root, "A.strm"))
	if string(raw) != string(newer) {
		t.Fatalf("published target missing after durability failure: %q", raw)
	}
	onlyEntryNamed(t, root, "A.strm")
}

// A crash after the exchange but before verification leaves the new target and
// the displaced old content under the deterministic staging name.
func TestUpdateInterruptedBeforeVerificationLeavesRecoverableState(t *testing.T) {
	root := t.TempDir()
	old, newer := writeUpdateFixture(t, root)
	crash := LocalFS{afterExchange: func(*securepath.Dir, string) error {
		return errors.New("simulated interruption")
	}}
	if err := crash.Update(root, "A.strm", old, newer); err == nil {
		t.Fatal("interruption must abort the update")
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "A.strm")); string(raw) != string(newer) {
		t.Fatalf("new bytes not durable at interruption: %q", raw)
	}
	if got := residualBytes(t, root, "A.strm"); string(got) != string(old) {
		t.Fatalf("displaced old content not preserved: %q", got)
	}

	// Recovery restores the pre-update content to the target so the normal
	// comparison can decide to update again.
	if err := (LocalFS{}).Recover(root, "A.strm", newer, true, false); err != nil {
		t.Fatalf("recover: %v", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "A.strm")); string(raw) != string(old) {
		t.Fatalf("recover did not restore old content: %q", raw)
	}
	onlyEntryNamed(t, root, "A.strm")
}

// A foreign target displaced by an interrupted exchange is restored to the
// target by recovery, is never deleted, and is reported as a conservative
// conflict so the normal comparison cannot silently overwrite it.
func TestRecoverRestoresForeignDisplacedContent(t *testing.T) {
	root := t.TempDir()
	_, newer := writeUpdateFixture(t, root)
	if err := os.WriteFile(filepath.Join(root, "A.strm"), newer, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, newStagingName("A.strm", []byte("previous-owned"), newer)), []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (LocalFS{}).Recover(root, "A.strm", newer, true, false); !errors.Is(err, ErrStagingPresent) {
		t.Fatalf("recover = %v, want ErrStagingPresent", err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "A.strm"))
	if string(raw) != "external" {
		t.Fatalf("foreign bytes were not restored to the target: %q", raw)
	}
	onlyEntryNamed(t, root, "A.strm")
}

func TestRecoverAmbiguousResidualIsConflictAndLeavesEverything(t *testing.T) {
	root := t.TempDir()
	old, newer := writeUpdateFixture(t, root)
	if err := os.WriteFile(filepath.Join(root, newStagingName("A.strm", old, newer)), []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (LocalFS{}).Recover(root, "A.strm", newer, true, false); !errors.Is(err, ErrStagingPresent) {
		t.Fatalf("recover = %v, want ErrStagingPresent", err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "A.strm"))
	if string(raw) != string(old) {
		t.Fatalf("ambiguous recovery changed the target: %q", raw)
	}
	if got := residualBytes(t, root, "A.strm"); string(got) != "foreign" {
		t.Fatalf("ambiguous recovery deleted the residual: %q", got)
	}
}

func TestRecoverRefusesUnownedStagingWithoutDeleting(t *testing.T) {
	root := t.TempDir()
	old, newer := writeUpdateFixture(t, root)
	if err := os.WriteFile(filepath.Join(root, newStagingName("A.strm", old, newer)), []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (LocalFS{}).Recover(root, "A.strm", newer, false, false); !errors.Is(err, ErrStagingPresent) {
		t.Fatalf("recover = %v, want ErrStagingPresent", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "A.strm")); string(raw) != string(old) {
		t.Fatalf("unowned recovery changed the target: %q", raw)
	}
	if got := residualBytes(t, root, "A.strm"); string(got) != "foreign" {
		t.Fatalf("unowned recovery deleted the residual: %q", got)
	}
}

func TestRecoverDryRunDetectsWithoutModifying(t *testing.T) {
	root := t.TempDir()
	old, newer := writeUpdateFixture(t, root)
	staging := filepath.Join(root, newStagingName("A.strm", old, newer))
	if err := os.WriteFile(staging, []byte("foreign"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (LocalFS{}).Recover(root, "A.strm", newer, true, true); !errors.Is(err, ErrStagingPresent) {
		t.Fatalf("dry-run recover = %v, want ErrStagingPresent", err)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "A.strm")); string(raw) != string(old) {
		t.Fatalf("dry-run recover changed the target: %q", raw)
	}
	if raw, _ := os.ReadFile(staging); string(raw) != "foreign" {
		t.Fatalf("dry-run recover changed the residual: %q", raw)
	}
}

func TestRecoverRemovesOwnStagingWhenTargetAbsent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	newer := []byte("# strm-probe-version:1\nhttps://e.invalid/d/A.mkv\n")
	if err := os.WriteFile(filepath.Join(root, newStagingName("A.strm", []byte("old"), newer)), newer, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (LocalFS{}).Recover(root, "A.strm", newer, true, false); err != nil {
		t.Fatalf("recover: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("own staging was not removed: %v %v", entries, err)
	}
}
