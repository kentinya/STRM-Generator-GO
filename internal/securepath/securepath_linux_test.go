//go:build linux

package securepath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenDirRejectsSymlinkAncestor(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenDir(filepath.Join(alias, "out"), true, 0o755); err == nil {
		t.Fatal("symlinked ancestor must be rejected")
	}
	if _, err := os.Stat(filepath.Join(real, "out")); !os.IsNotExist(err) {
		t.Fatal("rejected path created a directory under the symlink target")
	}
}

func TestOpenDirHandleSurvivesPathReplacementWithoutEscape(t *testing.T) {
	base := t.TempDir()
	rootPath := filepath.Join(base, "out")
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := OpenDir(rootPath, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	moved := filepath.Join(base, "detached-output")
	if err := os.Rename(rootPath, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, rootPath); err != nil {
		t.Fatal(err)
	}
	if err := root.Mkdir("inside", 0o755); err != nil {
		t.Fatalf("descriptor-relative operation failed: %v", err)
	}
	if info, err := os.Stat(filepath.Join(moved, "inside")); err != nil || !info.IsDir() {
		t.Fatalf("operation did not stay on the opened directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "inside")); !os.IsNotExist(err) {
		t.Fatal("operation escaped into the replacement symlink target")
	}
}

func TestValidateDirPathAllowsMissingSuffixButRejectsSymlink(t *testing.T) {
	base := t.TempDir()
	missing := filepath.Join(base, "missing", "out")
	if err := ValidateDirPath(missing); err != nil {
		t.Fatalf("missing output suffix should be valid: %v", err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(base, link); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDirPath(filepath.Join(link, "out")); err == nil {
		t.Fatal("symlinked ancestor should be rejected")
	}
}

func TestBoundDirRejectsLateSymlinkForMissingRoot(t *testing.T) {
	base := t.TempDir()
	rootPath := filepath.Join(base, "out")
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	bound, err := BindDir(rootPath)
	if err != nil {
		t.Fatalf("bind missing root: %v", err)
	}
	defer bound.Close()
	if err := os.Symlink(outside, rootPath); err != nil {
		t.Fatal(err)
	}
	if _, err := bound.Open(true, 0o755); err == nil {
		t.Fatal("late symlink must not be followed")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("outside target changed: entries=%v err=%v", entries, err)
	}
}

func TestBoundDirDetectsReplacementOfExistingRoot(t *testing.T) {
	base := t.TempDir()
	rootPath := filepath.Join(base, "out")
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(rootPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	bound, err := BindDir(rootPath)
	if err != nil {
		t.Fatalf("bind root: %v", err)
	}
	defer bound.Close()
	if err := bound.VerifyPath(); err != nil {
		t.Fatalf("unchanged root did not verify: %v", err)
	}
	moved := filepath.Join(base, "moved-out")
	if err := os.Rename(rootPath, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, rootPath); err != nil {
		t.Fatal(err)
	}
	if err := bound.VerifyPath(); err == nil {
		t.Fatal("replacement root must not verify as the bound directory")
	}
	dir, err := bound.Open(false, 0)
	if err != nil {
		t.Fatalf("bound descriptor should remain usable: %v", err)
	}
	defer dir.Close()
	if err := dir.Mkdir("inside", 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(moved, "inside")); err != nil {
		t.Fatalf("descriptor no longer refers to the original root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "inside")); !os.IsNotExist(err) {
		t.Fatal("descriptor operation escaped to replacement symlink target")
	}
}

func TestPathsOverlapUsesDirectoryHierarchyAndRejectsAliases(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "out")
	nested := filepath.Join(root, "nested")
	sibling := filepath.Join(base, "other")
	for _, dir := range []string{root, nested, sibling} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if overlap, err := PathsOverlap(root, nested); err != nil || !overlap {
		t.Fatalf("nested directories overlap: overlap=%t err=%v", overlap, err)
	}
	if overlap, err := PathsOverlap(root, sibling); err != nil || overlap {
		t.Fatalf("sibling directories should not overlap: overlap=%t err=%v", overlap, err)
	}
	missingChild := filepath.Join(root, "new")
	if overlap, err := PathsOverlap(root, missingChild); err != nil || !overlap {
		t.Fatalf("missing descendant should overlap: overlap=%t err=%v", overlap, err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := PathsOverlap(root, filepath.Join(alias, "nested")); err == nil {
		t.Fatal("symlink path alias must be rejected rather than followed")
	}
}

func TestDirectoryContainsPathHandlesDatabaseSidecarAndSibling(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "out")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path string
		want bool
	}{
		{path: filepath.Join(root, "db.sqlite"), want: true},
		{path: filepath.Join(root, "db.sqlite-wal"), want: true},
		{path: filepath.Join(base, "db.sqlite"), want: false},
	} {
		got, err := DirectoryContainsPath(root, tc.path)
		if err != nil || got != tc.want {
			t.Fatalf("contains(%q)=%t err=%v, want %t", tc.path, got, err, tc.want)
		}
	}
}
