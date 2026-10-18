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
