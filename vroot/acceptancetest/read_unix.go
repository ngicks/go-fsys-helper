//go:build unix

package acceptancetest

import (
	"errors"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// Unix-specific writeFails test with proper chmod expectations
func writeFails(t *testing.T, fsys vroot.Fs) {
	// Test filesystem-level write operations that should fail

	// Create should fail
	_, err := fsys.Create("should_fail.txt")
	if err == nil {
		t.Error("Create should have failed on read-only filesystem")
	}

	// OpenFile with write flags should fail
	_, err = fsys.OpenFile("file1.txt", os.O_RDWR, 0o644)
	if err == nil {
		t.Error("OpenFile with O_RDWR should have failed on read-only filesystem")
	}

	// Test Unix-specific chmod behavior
	testWriteFailsChmod(t, fsys)

	// Chtimes should fail
	err = fsys.Chtimes("file1.txt", time.Now(), time.Now())
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Error("Chtimes should have failed on read-only filesystem")
	}

	// Mkdir should fail
	err = fsys.Mkdir("new_dir", 0o755)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Error("Mkdir should have failed on read-only filesystem")
	}

	// MkdirAll should fail
	err = fsys.MkdirAll("new/deep/dir", 0o755)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Error("MkdirAll should have failed on read-only filesystem")
	}

	// Symlink should fail
	err = fsys.Symlink("file1.txt", "new_symlink")
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Error("Symlink should have failed on read-only filesystem")
	}

	// Link should fail
	err = fsys.Link("file1.txt", "new_hardlink")
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Error("Link should have failed on read-only filesystem")
	}

	// Remove should fail
	err = fsys.Remove("file1.txt")
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Error("Remove should have failed on read-only filesystem")
	}

	// RemoveAll should fail
	err = fsys.RemoveAll("subdir")
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Error("RemoveAll should have failed on read-only filesystem")
	}

	// Rename should fail
	err = fsys.Rename("file1.txt", "renamed.txt")
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		t.Error("Rename should have failed on read-only filesystem")
	}

	// Test file-level write operations on opened files
	f, err := fsys.Open("file1.txt")
	if err != nil {
		t.Fatalf("Open file1.txt failed: %v", err)
	}
	defer f.Close()

	// Write should fail
	_, err = f.Write([]byte("test"))
	if err == nil {
		t.Error("File.Write should have failed on read-only filesystem")
	}

	// WriteString should fail
	_, err = f.WriteString("test")
	if err == nil {
		t.Error("File.WriteString should have failed on read-only filesystem")
	}

	// WriteAt should fail (or return ErrOpNotSupported)
	_, err = f.WriteAt([]byte("test"), 0)
	if err == nil {
		t.Error("File.WriteAt should have failed on read-only filesystem")
	} else if errors.Is(err, vroot.ErrOpNotSupported) {
		t.Logf("File.WriteAt not supported (ErrOpNotSupported)")
	}

	// Truncate should fail
	err = f.Truncate(0)
	if err == nil {
		t.Error("File.Truncate should have failed on read-only filesystem")
	}

	// Test Unix-specific file chmod behavior
	testFileWriteFailsChmod(t, f)
}
