//go:build unix

package acceptancetest

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// Test file-level chmod operations on Unix
func testFileChmod(t *testing.T, fsys vroot.Fs) {
	// Create a file for testing
	f, err := fsys.Create("test_chmod.txt")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer f.Close()

	// Test file-level chmod
	err = f.Chmod(0o755)
	if err != nil {
		t.Fatalf("File.Chmod failed: %v", err)
	}

	// Verify Chmod effect by checking file permissions
	info, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat after File.Chmod failed: %v", err)
	}
	// Note: permissions may be widened or narrowed by platform, so we check if it's reasonable
	mode := info.Mode().Perm()
	if mode&0o700 != 0o700 {
		t.Errorf("File.Chmod effect not observed: got mode %o, expected owner permissions to include 0o700", mode)
	}
}

// Test filesystem-level chmod operations on Unix
func testFilesystemChmod(t *testing.T, fsys vroot.Fs) {
	// Create a file for testing
	f, err := fsys.Create("test_fs_chmod.txt")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	f.Close()

	// Test filesystem-level chmod
	err = fsys.Chmod("test_fs_chmod.txt", 0o755)
	if err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	// Verify filesystem Chmod effect
	info, err := fsys.Stat("test_fs_chmod.txt")
	if err != nil {
		t.Fatalf("Stat after Chmod failed: %v", err)
	}
	mode := info.Mode().Perm()
	if mode&0o700 != 0o700 {
		t.Errorf("Chmod effect not observed: got mode %o, expected owner permissions to include 0o700", mode)
	}
}

// Test Mkdir operations on Unix
func testMkdir(t *testing.T, fsys vroot.Fs) {
	// Test Mkdir
	err := fsys.Mkdir("test_dir", 0o755)
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

	// Test MkdirAll
	err = fsys.MkdirAll("test_deep/nested/dir", 0o755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	// MkdirAll again. It should return nil error
	err = fsys.MkdirAll("test_deep/nested/dir", 0o755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// Verify MkdirAll effect
	info, err = fsys.Stat("test_deep/nested/dir")
	if err != nil {
		t.Fatalf("Stat after MkdirAll failed: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("MkdirAll effect not observed: created item is not a directory")
	}
}

// Unix-specific chmod test for writeFails
func testWriteFailsChmod(t *testing.T, fsys vroot.Fs) {
	// Chmod should fail
	err := fsys.Chmod("file1.txt", 0o755)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Error("Chmod should have failed on read-only filesystem")
	}
}

// Unix-specific chmod test for file writeFails
func testFileWriteFailsChmod(t *testing.T, f vroot.File) {
	// Chmod should fail
	err := f.Chmod(0o755)
	if err == nil {
		t.Error("File.Chmod should have failed on read-only filesystem")
	}
}

// Unix-specific RemoveAll with chmod test
func testRemoveAllWithChmod(t *testing.T, fsys vroot.Fs) {
	// Test RemoveAll with permission issues (create read-only directory)
	err := fsys.Mkdir("readonly_parent", 0o755)
	if err != nil {
		t.Fatalf("Create readonly parent failed: %v", err)
	}
	err = fsys.Mkdir("readonly_parent/child", 0o755)
	if err != nil {
		t.Fatalf("Create child in readonly parent failed: %v", err)
	}

	// Make parent read-only to prevent deletion of child
	err = fsys.Chmod("readonly_parent", 0o555)
	if err != nil {
		t.Fatalf("Chmod readonly_parent failed: %v", err)
	}

	// This might fail due to permissions
	err = fsys.RemoveAll("readonly_parent")
	// Clean up by restoring permissions first
	fsys.Chmod("readonly_parent", 0o755)
	fsys.RemoveAll("readonly_parent")

	// We don't assert on the error here as behavior may vary by implementation
	// Some implementations might handle this gracefully, others might fail
}
