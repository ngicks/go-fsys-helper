package acceptancetest

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// populates writable fsys using [RootFsys].
func populateRoot(t *testing.T, fsys vroot.Fs) {
	for _, txt := range RootFsys {
		var ok bool
		txt, ok = strings.CutPrefix(txt, "root/readable/")
		if !ok {
			continue
		}

		switch {
		case strings.HasSuffix(txt, "/"):
			err := fsys.Mkdir(filepath.FromSlash(txt), fs.ModePerm)
			if err != nil && !errors.Is(err, fs.ErrExist) {
				t.Fatalf("mkdir %q failed with %v", txt, err)
			}
		case strings.Contains(txt, ": "):
			idx := strings.Index(txt, ": ")
			path := txt[:idx]
			content := txt[idx+len(": "):]
			f, err := fsys.OpenFile(filepath.FromSlash(path), os.O_CREATE|os.O_RDWR, fs.ModePerm)
			if err != nil {
				t.Fatalf("open %q failed with %v", path, err)
			}
			_, err = f.Write([]byte(content))
			f.Close()
			if err != nil {
				t.Fatalf("write %q failed with %v", path, err)
			}
		case strings.Contains(txt, " -> "):
			idx := strings.Index(txt, " -> ")
			path := txt[:idx]
			target := txt[idx+len(" -> "):]
			err := fsys.Symlink(filepath.FromSlash(target), filepath.FromSlash(path))
			if err != nil {
				t.Fatalf("symlink %q -> %q failed with %v", path, target, err)
			}
		}
	}
}

// call every write methods, e.g. Chmod, Chtime, OpenFile with [os.O_RDWR], Create, etc.
// Also call every write methods on vroot.File
//
// All write operations must succeeds if they should.
func write(t *testing.T, fsys vroot.Fs) {
	// Test filesystem-level write operations

	// Create a new file
	f, err := fsys.Create("test_write.txt")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer f.Close()

	// Write to the file
	content := "test content"
	n, err := f.Write([]byte(content))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(content) {
		t.Fatalf("Write wrote %d bytes, expected %d", n, len(content))
	}

	// Test file-level write operations
	err = f.Chmod(0o644)
	if err != nil {
		t.Fatalf("File.Chmod failed: %v", err)
	}

	// Close the file and reopen for more tests
	f.Close()

	// Test OpenFile with write flags
	f2, err := fsys.OpenFile("test_write2.txt", os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("OpenFile with O_RDWR failed: %v", err)
	}
	defer f2.Close()

	// Write using WriteString
	n2, err := f2.WriteString("test string")
	if err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}
	if n2 != len("test string") {
		t.Fatalf("WriteString wrote %d bytes, expected %d", n2, len("test string"))
	}

	// Test Truncate
	err = f2.Truncate(4)
	if err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	// Test filesystem-level operations
	err = fsys.Chmod("test_write.txt", 0o755)
	if err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}

	err = fsys.Chtimes("test_write.txt", time.Now(), time.Now())
	if err != nil {
		t.Fatalf("Chtimes failed: %v", err)
	}

	// Test Mkdir
	err = fsys.Mkdir("test_dir", 0o755)
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	// Test MkdirAll
	err = fsys.MkdirAll("test_deep/nested/dir", 0o755)
	if err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// Test Symlink (create a symlink that doesn't escape)
	err = fsys.Symlink("test_write.txt", "test_symlink")
	if err != nil {
		t.Fatalf("Symlink failed: %v", err)
	}

	// Test Link (hard link)
	err = fsys.Link("test_write.txt", "test_hardlink")
	if err != nil {
		t.Fatalf("Link failed: %v", err)
	}

	// Test Rename
	err = fsys.Rename("test_write2.txt", "test_renamed.txt")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}
}

// test implementation of vroot.File as regular file.
// ReadDir* methods error for a regular file.
//
// Run same test for every file.
func readFile(t *testing.T, fsys vroot.Fs) {
	files := []string{"file1.txt", "file2.txt", "subdir/nested_file.txt"}
	expectedContents := []string{"bazbazbaz", "quxquxqux", "nested_file"}

	for i, filename := range files {
		t.Run(filename, func(t *testing.T) {
			// Test Open
			f, err := fsys.Open(filename)
			if err != nil {
				t.Fatalf("Open %q failed: %v", filename, err)
			}
			defer f.Close()

			// Test Read
			buf := make([]byte, 1024)
			n, err := f.Read(buf)
			if err != nil && err != io.EOF {
				t.Fatalf("Read %q failed: %v", filename, err)
			}
			content := string(buf[:n])
			if content != expectedContents[i] {
				t.Errorf("Read %q got %q, expected %q", filename, content, expectedContents[i])
			}

			// Test Stat
			info, err := f.Stat()
			if err != nil {
				t.Fatalf("Stat %q failed: %v", filename, err)
			}
			if info.IsDir() {
				t.Errorf("Stat %q reported as directory, should be file", filename)
			}

			// Test ReadDir should fail for regular files
			_, err = f.ReadDir(-1)
			if err == nil {
				t.Errorf("ReadDir on file %q should have failed", filename)
			}

			// Test Readdir should fail for regular files
			_, err = f.Readdir(-1)
			if err == nil {
				t.Errorf("Readdir on file %q should have failed", filename)
			}

			// Test Readdirnames should fail for regular files
			_, err = f.Readdirnames(-1)
			if err == nil {
				t.Errorf("Readdirnames on file %q should have failed", filename)
			}

			// Test Seek
			offset, err := f.Seek(0, io.SeekStart)
			if err != nil {
				t.Fatalf("Seek %q failed: %v", filename, err)
			}
			if offset != 0 {
				t.Errorf("Seek %q returned %d, expected 0", filename, offset)
			}

			// Test ReadAt
			buf2 := make([]byte, 3)
			n2, err := f.ReadAt(buf2, 0)
			if err != nil && err != io.EOF {
				t.Fatalf("ReadAt %q failed: %v", filename, err)
			}
			if n2 > 0 && string(buf2[:n2]) != expectedContents[i][:n2] {
				t.Errorf("ReadAt %q got %q, expected %q", filename, string(buf2[:n2]), expectedContents[i][:n2])
			}
		})
	}
}

// test implementation of vroot.File as directory.
// see regular file fails for read dir operation but directory returns correct result.
//
// Run same test for every directory.
func readDirectory(t *testing.T, fsys vroot.Fs) {
	dirs := []string{".", "subdir", "subdir/double_nested"}

	for _, dirname := range dirs {
		t.Run(dirname, func(t *testing.T) {
			// Test Open
			f, err := fsys.Open(dirname)
			if err != nil {
				t.Fatalf("Open directory %q failed: %v", dirname, err)
			}
			defer f.Close()

			// Test Stat
			info, err := f.Stat()
			if err != nil {
				t.Fatalf("Stat directory %q failed: %v", dirname, err)
			}
			if !info.IsDir() {
				t.Errorf("Stat %q reported as file, should be directory", dirname)
			}

			// Test ReadDir
			entries, err := f.ReadDir(-1)
			if err != nil {
				t.Fatalf("ReadDir %q failed: %v", dirname, err)
			}
			if len(entries) == 0 {
				t.Errorf("ReadDir %q returned no entries", dirname)
			}

			// Verify entries have names
			for _, entry := range entries {
				if entry.Name() == "" {
					t.Errorf("ReadDir %q returned entry with empty name", dirname)
				}
			}

			// Test Readdir
			f.Seek(0, io.SeekStart) // Reset position
			infos, err := f.Readdir(-1)
			if err != nil {
				t.Fatalf("Readdir %q failed: %v", dirname, err)
			}
			if len(infos) != len(entries) {
				t.Errorf("Readdir %q returned %d entries, ReadDir returned %d", dirname, len(infos), len(entries))
			}

			// Test Readdirnames
			f.Seek(0, io.SeekStart) // Reset position
			names, err := f.Readdirnames(-1)
			if err != nil {
				t.Fatalf("Readdirnames %q failed: %v", dirname, err)
			}
			if len(names) != len(entries) {
				t.Errorf("Readdirnames %q returned %d names, ReadDir returned %d entries", dirname, len(names), len(entries))
			}

			// Verify names match
			for i, name := range names {
				if i < len(entries) && name != entries[i].Name() {
					t.Errorf("Readdirnames[%d] = %q, ReadDir[%d].Name() = %q", i, name, i, entries[i].Name())
				}
			}

			// Test Read should fail for directories
			buf := make([]byte, 1024)
			_, err = f.Read(buf)
			if err == nil {
				t.Errorf("Read on directory %q should have failed", dirname)
			}
		})
	}
}

// call every write methods, e.g. Chmod, Chtime, OpenFile with [os.O_RDWR], Create, etc.
// Also call every write methods on vroot.File
// all write operation must fails.
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

	// Chmod should fail
	err = fsys.Chmod("file1.txt", 0o755)
	if err == nil {
		t.Error("Chmod should have failed on read-only filesystem")
	}

	// Chtimes should fail
	err = fsys.Chtimes("file1.txt", time.Now(), time.Now())
	if err == nil {
		t.Error("Chtimes should have failed on read-only filesystem")
	}

	// Mkdir should fail
	err = fsys.Mkdir("new_dir", 0o755)
	if err == nil {
		t.Error("Mkdir should have failed on read-only filesystem")
	}

	// MkdirAll should fail
	err = fsys.MkdirAll("new/deep/dir", 0o755)
	if err == nil {
		t.Error("MkdirAll should have failed on read-only filesystem")
	}

	// Symlink should fail
	err = fsys.Symlink("file1.txt", "new_symlink")
	if err == nil {
		t.Error("Symlink should have failed on read-only filesystem")
	}

	// Link should fail
	err = fsys.Link("file1.txt", "new_hardlink")
	if err == nil {
		t.Error("Link should have failed on read-only filesystem")
	}

	// Remove should fail
	err = fsys.Remove("file1.txt")
	if err == nil {
		t.Error("Remove should have failed on read-only filesystem")
	}

	// RemoveAll should fail
	err = fsys.RemoveAll("subdir")
	if err == nil {
		t.Error("RemoveAll should have failed on read-only filesystem")
	}

	// Rename should fail
	err = fsys.Rename("file1.txt", "renamed.txt")
	if err == nil {
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

	// WriteAt should fail
	_, err = f.WriteAt([]byte("test"), 0)
	if err == nil {
		t.Error("File.WriteAt should have failed on read-only filesystem")
	}

	// Truncate should fail
	err = f.Truncate(0)
	if err == nil {
		t.Error("File.Truncate should have failed on read-only filesystem")
	}

	// Chmod should fail
	err = f.Chmod(0o755)
	if err == nil {
		t.Error("File.Chmod should have failed on read-only filesystem")
	}
}

// test symlink resolusion.
// Lstat indicates files as symlink if it is.
// ReadLink succeeds.
func followSymlink(t *testing.T, fsys vroot.Fs) {
	symlinks := map[string]string{
		"symlink_inner":         "./file1.txt",
		"symlink_inner_dir":     "./subdir",
		"subdir/symlink_upward": "../symlink_inner",
	}

	for linkName, target := range symlinks {
		t.Run(linkName, func(t *testing.T) {
			// Test Lstat shows it's a symlink
			info, err := fsys.Lstat(linkName)
			if err != nil {
				t.Fatalf("Lstat %q failed: %v", linkName, err)
			}
			if info.Mode()&fs.ModeSymlink == 0 {
				t.Errorf("Lstat %q should show symlink mode", linkName)
			}

			// Test Readlink returns correct target
			readTarget, err := fsys.Readlink(linkName)
			if err != nil {
				t.Fatalf("Readlink %q failed: %v", linkName, err)
			}
			if readTarget != target {
				t.Errorf("Readlink %q got %q, expected %q", linkName, readTarget, target)
			}

			// Test that we can follow the symlink (Open should work)
			f, err := fsys.Open(linkName)
			if err != nil {
				t.Fatalf("Open symlink %q failed: %v", linkName, err)
			}
			f.Close()

			// Test Stat (follows symlink) vs Lstat (doesn't follow)
			statInfo, err := fsys.Stat(linkName)
			if err != nil {
				t.Fatalf("Stat %q failed: %v", linkName, err)
			}

			// Stat should show the target's properties, not the symlink
			lstatInfo, err := fsys.Lstat(linkName)
			if err != nil {
				t.Fatalf("Lstat %q failed: %v", linkName, err)
			}

			// The mode should be different (Stat follows, Lstat doesn't)
			if statInfo.Mode()&fs.ModeSymlink != 0 {
				t.Errorf("Stat %q should not show symlink mode (should follow link)", linkName)
			}
			if lstatInfo.Mode()&fs.ModeSymlink == 0 {
				t.Errorf("Lstat %q should show symlink mode (should not follow link)", linkName)
			}
		})
	}
}
