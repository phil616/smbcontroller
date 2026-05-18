package smb

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestEnsureShareTreePermissionsRecursivelyFixesExistingEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions are not supported on windows")
	}

	root := t.TempDir()
	childDir := filepath.Join(root, "child")
	oldFile := filepath.Join(root, "old.txt")
	childFile := filepath.Join(childDir, "nested.txt")
	linkPath := filepath.Join(root, "link.txt")
	targetPath := filepath.Join(root, "target.txt")

	if err := os.Mkdir(childDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldFile, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(childFile, []byte("nested"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("target"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Fatal(err)
	}

	gid := os.Getgid()
	if err := ensureShareTreePermissions(root, gid); err != nil {
		t.Fatal(err)
	}

	assertMode(t, root, 0770, 0770)
	assertMode(t, childDir, 0770, 0770)
	assertSetGID(t, root)
	assertSetGID(t, childDir)
	assertMode(t, oldFile, 0660, 0660)
	assertMode(t, childFile, 0660, 0660)
	assertGID(t, root, gid)
	assertGID(t, childDir, gid)
	assertGID(t, oldFile, gid)
	assertGID(t, childFile, gid)
}

func TestEnsureShareTreePermissionsGrantsOwnerBitsOnLockedDownFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions are not supported on windows")
	}

	root := t.TempDir()
	// Files created by other users sometimes land with no owner permissions
	// at all (e.g. 0040, 0004). The previous implementation only ORed in
	// group rw, so over SMB the original owner still saw "permission denied"
	// when reading their own file — group bits are ignored once the access
	// uid matches the owner uid.
	ownerLocked := filepath.Join(root, "owner_locked.txt")
	if err := os.WriteFile(ownerLocked, []byte("locked"), 0040); err != nil {
		t.Fatal(err)
	}

	if err := ensureShareTreePermissions(root, os.Getgid()); err != nil {
		t.Fatal(err)
	}

	assertMode(t, ownerLocked, 0777, 0660)
}

func TestEnsureShareTreePermissionsSkipsDanglingSymlinksAndFixesSiblings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions are not supported on windows")
	}

	root := t.TempDir()
	good := filepath.Join(root, "good.txt")
	if err := os.WriteFile(good, []byte("good"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "does-not-exist"), filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}

	if err := ensureShareTreePermissions(root, os.Getgid()); err != nil {
		t.Fatalf("walk aborted unexpectedly: %v", err)
	}

	assertMode(t, good, 0660, 0660)
}

func assertMode(t *testing.T, path string, mask, want os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode() & mask; got != want {
		t.Fatalf("%s mode bits = %v, want %v", path, got, want)
	}
}

func assertGID(t *testing.T, path string, want int) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("%s stat type = %T, want *syscall.Stat_t", path, info.Sys())
	}
	if got := int(stat.Gid); got != want {
		t.Fatalf("%s gid = %d, want %d", path, got, want)
	}
}

func assertSetGID(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSetgid == 0 {
		t.Fatalf("%s mode does not include setgid: %v", path, info.Mode())
	}
}
