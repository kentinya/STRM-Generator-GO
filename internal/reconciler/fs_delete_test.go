package reconciler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"strm-generator-go/internal/securepath"
)

func TestDeleteOwnedRemovesRegularTarget(t *testing.T) {
	root := t.TempDir()
	fs := LocalFS{}
	if err := os.WriteFile(filepath.Join(root, "A.strm"), []byte("doc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fs.InspectDeletable(context.Background(), root, "A.strm"); err != nil {
		t.Fatalf("inspect: %v", err)
	}
	outcome, err := fs.DeleteOwned(context.Background(), root, "A.strm")
	if err != nil || !outcome.Removed {
		t.Fatalf("delete: outcome=%+v err=%v", outcome, err)
	}
	if _, err := os.Stat(filepath.Join(root, "A.strm")); !os.IsNotExist(err) {
		t.Fatal("target was not removed")
	}
}

func TestDeleteOwnedHandlesAlreadyAbsent(t *testing.T) {
	root := t.TempDir()
	fs := LocalFS{}
	outcome, err := fs.DeleteOwned(context.Background(), root, "A.strm")
	if err != nil || !outcome.AlreadyAbsent || outcome.Removed {
		t.Fatalf("absent target: outcome=%+v err=%v", outcome, err)
	}
	// A missing parent directory is equivalent to an absent target.
	outcome, err = fs.DeleteOwned(context.Background(), root, "sub/A.strm")
	if err != nil || !outcome.AlreadyAbsent {
		t.Fatalf("absent parent: outcome=%+v err=%v", outcome, err)
	}
}

func TestDeleteOwnedRefusesSymlinkSpecialAndDirectory(t *testing.T) {
	root := t.TempDir()
	fs := LocalFS{}
	outside := filepath.Join(t.TempDir(), "sentinel")
	if err := os.WriteFile(outside, []byte("sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.strm")); err != nil {
		t.Fatal(err)
	}
	if outcome, err := fs.DeleteOwned(context.Background(), root, "link.strm"); !errors.Is(err, ErrNotDeletable) || outcome.Removed {
		t.Fatalf("symlink: outcome=%+v err=%v", outcome, err)
	}
	if raw, err := os.ReadFile(outside); err != nil || string(raw) != "sentinel" {
		t.Fatalf("symlink target changed: %q err=%v", raw, err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "fifo.strm"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.DeleteOwned(context.Background(), root, "fifo.strm"); !errors.Is(err, ErrNotDeletable) {
		t.Fatalf("FIFO: err=%v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "dir.strm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.DeleteOwned(context.Background(), root, "dir.strm"); !errors.Is(err, ErrNotDeletable) {
		t.Fatalf("directory: err=%v", err)
	}
}

func TestDeleteOwnedRefusesStagingResidual(t *testing.T) {
	root := t.TempDir()
	fs := LocalFS{}
	target := filepath.Join(root, "A.strm")
	if err := os.WriteFile(target, []byte("doc"), 0o644); err != nil {
		t.Fatal(err)
	}
	residual := filepath.Join(root, stagingPrefix("A.strm")+"00000000000000000000000000000000"+updateStagingSuffix)
	if err := os.WriteFile(residual, []byte("residual"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := fs.InspectDeletable(context.Background(), root, "A.strm"); !errors.Is(err, ErrStagingPresent) {
		t.Fatalf("inspect staging: err=%v", err)
	}
	if _, err := fs.DeleteOwned(context.Background(), root, "A.strm"); !errors.Is(err, ErrStagingPresent) {
		t.Fatalf("delete staging: err=%v", err)
	}
	if raw, err := os.ReadFile(target); err != nil || string(raw) != "doc" {
		t.Fatalf("target changed: %q err=%v", raw, err)
	}
	if raw, err := os.ReadFile(residual); err != nil || string(raw) != "residual" {
		t.Fatalf("residual changed: %q err=%v", raw, err)
	}
}

func TestDeleteOwnedSyncFailureReportsDoneUnlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "A.strm")
	if err := os.WriteFile(target, []byte("doc"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := LocalFS{syncDir: func(*securepath.Dir) error { return errors.New("sync failure") }}
	outcome, err := fs.DeleteOwned(context.Background(), root, "A.strm")
	if err == nil || !outcome.Removed {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatal("unlink did not happen before the sync failure")
	}
}

func TestDeleteOwnedCancellationBeforeUnlinkPreservesTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "A.strm")
	if err := os.WriteFile(target, []byte("doc"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fs := LocalFS{}
	outcome, err := fs.DeleteOwned(ctx, root, "A.strm")
	if err == nil || outcome.Removed {
		t.Fatalf("cancelled delete: outcome=%+v err=%v", outcome, err)
	}
	if _, statErr := os.Stat(target); statErr != nil {
		t.Fatal("cancelled delete removed the target")
	}
}
