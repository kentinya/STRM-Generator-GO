package reconciler

import (
	"os"
	"path/filepath"
	"testing"
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

func TestLockIsExclusive(t *testing.T) {
	t.Setenv(lockEnv, t.TempDir())
	first, err := AcquireLock()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, err := AcquireLock(); err != ErrBusy {
		t.Fatalf("second acquire = %v, want ErrBusy", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	again, err := AcquireLock()
	if err != nil {
		t.Fatalf("re-acquire: %v", err)
	}
	again.Release()
}

func TestLockIsCrossProcessSafe(t *testing.T) {
	dir := t.TempDir()
	first, err := AcquireLockIn(dir)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer first.Release()
	if _, err := AcquireLockIn(dir); err != ErrBusy {
		t.Fatalf("second acquire = %v, want ErrBusy", err)
	}
}
