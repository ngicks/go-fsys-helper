package acceptancetest

import (
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// test implementation of vroot.File as regular file.
// ReadDir* methods error for a regular file.
//
// Run same test for every file.
func readFile(t *testing.T, fsys vroot.Fs) {
	files := []string{"file1.txt", "file2.txt", filepath.FromSlash("subdir/nested_file.txt")}
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
				if errors.Is(err, vroot.ErrOpNotSupported) {
					t.Logf("Seek %q not supported (ErrOpNotSupported)", filename)
				} else {
					t.Fatalf("Seek %q failed: %v", filename, err)
				}
			} else if offset != 0 {
				t.Errorf("Seek %q returned %d, expected 0", filename, offset)
			}

			// Test ReadAt
			buf2 := make([]byte, 3)
			n2, err := f.ReadAt(buf2, 0)
			if err != nil && err != io.EOF {
				if errors.Is(err, vroot.ErrOpNotSupported) {
					t.Logf("ReadAt %q not supported (ErrOpNotSupported)", filename)
				} else {
					t.Fatalf("ReadAt %q failed: %v", filename, err)
				}
			} else if n2 > 0 && string(buf2[:n2]) != expectedContents[i][:n2] {
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
	dirs := []string{".", "subdir", filepath.FromSlash("subdir/double_nested")}

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

			// Test Readdir - reopen file to reset position
			f2, err := fsys.Open(dirname)
			if err != nil {
				t.Fatalf("Reopen directory %q for Readdir failed: %v", dirname, err)
			}
			infos, err := f2.Readdir(-1)
			f2.Close()
			if err != nil {
				t.Fatalf("Readdir %q failed: %v", dirname, err)
			}
			if len(infos) != len(entries) {
				t.Errorf("Readdir %q returned %d entries, ReadDir returned %d", dirname, len(infos), len(entries))
			}

			// Test Readdirnames - reopen file to reset position
			f3, err := fsys.Open(dirname)
			if err != nil {
				t.Fatalf("Reopen directory %q for Readdirnames failed: %v", dirname, err)
			}
			names, err := f3.Readdirnames(-1)
			f3.Close()
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

// writeFails is implemented in platform-specific files:
// - read_unix.go for Unix systems
// - read_windows.go for Windows systems
// Tests that all write operations fail on read-only filesystem

// test symlink resolusion.
// Lstat indicates files as symlink if it is.
// ReadLink succeeds.
func followSymlink(t *testing.T, fsys vroot.Fs) {
	symlinks := map[string]string{
		"symlink_inner":     filepath.FromSlash("./file1.txt"),
		"symlink_inner_dir": filepath.FromSlash("./subdir"),
		filepath.FromSlash("subdir/symlink_upward"): filepath.FromSlash("../symlink_inner"),
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

			// Test ReadLink returns correct target
			readTarget, err := fsys.ReadLink(linkName)
			if err != nil {
				t.Fatalf("ReadLink %q failed: %v", linkName, err)
			}
			if readTarget != target {
				t.Errorf("ReadLink %q got %q, expected %q", linkName, readTarget, target)
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
