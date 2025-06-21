//go:build windows

package acceptancetest

import (
	"path/filepath"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// Test file-level chmod operations on Windows
func testFileChmod(t *testing.T, fsys vroot.Fs) {
	// Create a file for testing
	f, err := fsys.Create("test_chmod.txt")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer f.Close()

	// Test file-level chmod with Windows-compatible permissions
	err = f.Chmod(0o666) // Windows typically supports read/write permissions
	if err != nil {
		t.Fatalf("File.Chmod failed: %v", err)
	}

	// On Windows, we mainly check that chmod doesn't fail
	// rather than exact permission matching since Windows has a different permission model
	info, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat after File.Chmod failed: %v", err)
	}

	// Log the actual permissions for debugging
	mode := info.Mode().Perm()
	t.Logf("File chmod set 0o666, got %o", mode)
}

// Test filesystem-level chmod operations on Windows
func testFilesystemChmod(t *testing.T, fsys vroot.Fs) {
	// Create a file for testing
	f, err := fsys.Create("test_fs_chmod.txt")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	f.Close()

	// Test filesystem-level chmod with Windows-compatible permissions
	err = fsys.Chmod("test_fs_chmod.txt", 0o666)
	if err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	// On Windows, we mainly check that chmod doesn't fail
	info, err := fsys.Stat("test_fs_chmod.txt")
	if err != nil {
		t.Fatalf("Stat after Chmod failed: %v", err)
	}

	// Log the actual permissions for debugging
	mode := info.Mode().Perm()
	t.Logf("Filesystem chmod set 0o666, got %o", mode)
}

// Test Mkdir operations on Windows
func testMkdir(t *testing.T, fsys vroot.Fs) {
	// Test Mkdir with Windows-compatible permissions
	err := fsys.Mkdir("test_dir", 0o777) // Windows directories typically have full permissions
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	// Verify Mkdir effect
	info, err := fsys.Stat("test_dir")
	if err != nil {
		t.Fatalf("Stat after Mkdir failed: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("Mkdir effect not observed: created item is not a directory")
	}

	// Test MkdirAll with Windows path separators handled properly
	err = fsys.MkdirAll(filepath.FromSlash("test_deep/nested/dir"), 0o777)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	// MkdirAll again. It should return nil error
	err = fsys.MkdirAll(filepath.FromSlash("test_deep/nested/dir"), 0o777)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// Verify MkdirAll effect
	info, err = fsys.Stat(filepath.FromSlash("test_deep/nested/dir"))
	if err != nil {
		t.Fatalf("Stat after MkdirAll failed: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("MkdirAll effect not observed: created item is not a directory")
	}
}

// Windows-specific chmod test for writeFails
func testWriteFailsChmod(t *testing.T, fsys vroot.Fs) {
	// Chmod should fail on read-only filesystem
	err := fsys.Chmod("file1.txt", 0o666)
	if err == nil {
		t.Error("Chmod should have failed on read-only filesystem")
	}
}

// Windows-specific chmod test for file writeFails
func testFileWriteFailsChmod(t *testing.T, f vroot.File) {
	// Chmod should fail on read-only filesystem
	err := f.Chmod(0o666)
	if err == nil {
		t.Error("File.Chmod should have failed on read-only filesystem")
	}
}

// Windows-specific RemoveAll (no chmod test since Windows handles permissions differently)
func testRemoveAllWithChmod(t *testing.T, fsys vroot.Fs) {
	// On Windows, we skip the chmod permission test since the permission model is different
	// and readonly directories work differently
	t.Log("Skipping chmod-based RemoveAll test on Windows due to different permission model")
}
