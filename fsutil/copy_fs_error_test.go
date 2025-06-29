package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestCopyFs_ErrorPaths(t *testing.T) {
	t.Run("Copy walk error", func(t *testing.T) {
		// Create root directory
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		dstDir := filepath.Join(tempDir, "dst")

		// Create src and dst directories
		if err := os.Mkdir(srcDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create src dir: %v", err)
		}
		if err := os.Mkdir(dstDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create dst dir: %v", err)
		}

		// Create a directory that we'll make unreadable
		unreadableDir := filepath.Join(srcDir, "unreadable")
		if err := os.Mkdir(unreadableDir, 0o000); err != nil {
			t.Fatalf("failed to create unreadable dir: %v", err)
		}
		defer os.Chmod(unreadableDir, fs.ModePerm) // Cleanup

		// Set up filesystems
		srcFs := os.DirFS(srcDir)
		dstFs := &osfsLite{base: dstDir}

		// Create copy option
		opt := testCopyFsOption{}

		// Perform copy - should fail due to unreadable directory
		err := opt.CopyAll(dstFs, srcFs, ".")
		if err == nil {
			t.Error("expected error when copying unreadable directory")
		}
	})

	t.Run("CopyPath stat error", func(t *testing.T) {
		// Create root directory
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		dstDir := filepath.Join(tempDir, "dst")

		// Create src and dst directories
		if err := os.Mkdir(srcDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create src dir: %v", err)
		}
		if err := os.Mkdir(dstDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create dst dir: %v", err)
		}

		// Set up filesystems
		srcFs := os.DirFS(srcDir)
		dstFs := &osfsLite{base: dstDir}

		// Create copy option
		opt := testCopyFsOption{}

		// Try to copy non-existent file
		err := opt.CopyPath(dstFs, srcFs, ".", "nonexistent.txt")
		if err == nil {
			t.Error("expected error when copying non-existent file")
		}
	})

	t.Run("CopyPath mkdir error", func(t *testing.T) {
		// Create root directory
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		dstDir := filepath.Join(tempDir, "dst")

		// Create src and dst directories
		if err := os.Mkdir(srcDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create src dir: %v", err)
		}
		if err := os.Mkdir(dstDir, 0o444); err != nil { // Read-only dst
			t.Fatalf("failed to create dst dir: %v", err)
		}
		defer os.Chmod(dstDir, fs.ModePerm) // Cleanup

		// Create source file in subdirectory
		if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), fs.ModePerm); err != nil {
			t.Fatalf("failed to create source subdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "subdir", "file.txt"), []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to create source file: %v", err)
		}

		// Set up filesystems
		srcFs := os.DirFS(srcDir)
		dstFs := &osfsLite{base: dstDir}

		// Create copy option
		opt := testCopyFsOption{}

		// Try to copy - should fail when creating directory
		err := opt.CopyPath(dstFs, srcFs, ".", filepath.FromSlash("subdir/file.txt"))
		if err == nil {
			t.Error("expected error when creating directory in read-only filesystem")
		}
	})

	t.Run("copyEntry errors", func(t *testing.T) {
		// Create root directory
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		dstDir := filepath.Join(tempDir, "dst")

		// Create src and dst directories
		if err := os.Mkdir(srcDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create src dir: %v", err)
		}
		if err := os.Mkdir(dstDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create dst dir: %v", err)
		}

		// Test copyEntry with walk error
		opt := testCopyFsOption{}
		mockErr := errors.New("walk error")
		err := opt.copyEntry(&osfsLite{base: dstDir}, os.DirFS(srcDir), "path", "path", nil, mockErr)
		if err != mockErr {
			t.Errorf("expected walk error to be returned")
		}

		// Create a file that we'll make unreadable
		unreadableFile := filepath.Join(srcDir, "unreadable.txt")
		if err := os.WriteFile(unreadableFile, []byte("content"), 0o000); err != nil {
			t.Fatalf("failed to create unreadable file: %v", err)
		}
		defer os.Chmod(unreadableFile, 0o644) // Cleanup

		// Try to copy unreadable file
		info, _ := os.Stat(unreadableFile)
		err = opt.copyEntry(&osfsLite{base: dstDir}, os.DirFS(srcDir), "unreadable.txt", "unreadable.txt", info, nil)
		if err == nil {
			t.Error("expected error when copying unreadable file")
		}
	})

	t.Run("copyEntry with symlink", func(t *testing.T) {
		// Create root directory
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		dstDir := filepath.Join(tempDir, "dst")

		// Create src and dst directories
		if err := os.Mkdir(srcDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create src dir: %v", err)
		}
		if err := os.Mkdir(dstDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create dst dir: %v", err)
		}

		// Create a file and a symlink to it
		targetFile := filepath.Join(srcDir, "target.txt")
		if err := os.WriteFile(targetFile, []byte("target content"), 0o644); err != nil {
			t.Fatalf("failed to create target file: %v", err)
		}

		linkFile := filepath.Join(srcDir, "link.txt")
		if err := os.Symlink("target.txt", linkFile); err != nil {
			t.Fatalf("failed to create symlink: %v", err)
		}

		// Get symlink info
		linkInfo, err := os.Lstat(linkFile)
		if err != nil {
			t.Fatalf("failed to lstat symlink: %v", err)
		}

		// Set up filesystems with symlink support
		srcFs := &osfsLite{base: srcDir}
		dstFs := &osfsLite{base: dstDir}

		// Create copy option
		opt := testCopyFsOption{}

		// Copy symlink using copyEntry
		err = opt.copyEntry(dstFs, srcFs, "link.txt", "link.txt", linkInfo, nil)
		if err != nil {
			t.Fatalf("copyEntry failed: %v", err)
		}

		// Verify symlink was copied
		copiedLinkInfo, err := os.Lstat(filepath.Join(dstDir, "link.txt"))
		if err != nil {
			t.Fatalf("failed to lstat copied link: %v", err)
		}
		if copiedLinkInfo.Mode()&fs.ModeSymlink == 0 {
			t.Error("copied link is not a symlink")
		}

		// Verify symlink target
		target, err := os.Readlink(filepath.Join(dstDir, "link.txt"))
		if err != nil {
			t.Fatalf("failed to read link target: %v", err)
		}
		if target != "target.txt" {
			t.Errorf("link target mismatch: expected(%q) != actual(%q)", "target.txt", target)
		}
	})

	t.Run("Copy with Lstat error", func(t *testing.T) {
		// Create root directory
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		dstDir := filepath.Join(tempDir, "dst")

		// Create src and dst directories
		if err := os.Mkdir(srcDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create src dir: %v", err)
		}
		if err := os.Mkdir(dstDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create dst dir: %v", err)
		}

		// Create a broken symlink that will cause Lstat issues during walk
		if err := os.Symlink("nonexistent", filepath.Join(srcDir, "broken")); err != nil {
			t.Fatalf("failed to create broken symlink: %v", err)
		}

		// Make the directory unreadable after creating the symlink
		if err := os.Chmod(srcDir, 0o000); err != nil {
			t.Fatalf("failed to make src dir unreadable: %v", err)
		}
		defer os.Chmod(srcDir, fs.ModePerm) // Cleanup

		// Set up filesystems
		srcFs := os.DirFS(srcDir)
		dstFs := &osfsLite{base: dstDir}

		// Create copy option
		opt := testCopyFsOption{}

		// Perform copy - should fail due to permission issues
		err := opt.CopyAll(dstFs, srcFs, ".")
		if err == nil {
			t.Error("expected error when copying from unreadable directory")
		}
	})

	t.Run("copyEntry with symlink when src doesn't support ReadLink", func(t *testing.T) {
		// Create root directory
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		dstDir := filepath.Join(tempDir, "dst")

		// Create src and dst directories
		if err := os.Mkdir(srcDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create src dir: %v", err)
		}
		if err := os.Mkdir(dstDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create dst dir: %v", err)
		}

		// Create a symlink
		linkFile := filepath.Join(srcDir, "link.txt")
		if err := os.Symlink("target.txt", linkFile); err != nil {
			t.Fatalf("failed to create symlink: %v", err)
		}

		// Get symlink info
		linkInfo, err := os.Lstat(linkFile)
		if err != nil {
			t.Fatalf("failed to lstat symlink: %v", err)
		}

		// Set up filesystems where src doesn't support ReadLink (os.DirFS)
		srcFs := os.DirFS(srcDir)
		dstFs := &osfsLite{base: dstDir}

		// Create copy option
		opt := testCopyFsOption{}

		// Copy symlink using copyEntry - should ignore the symlink
		err = opt.copyEntry(dstFs, srcFs, "link.txt", "link.txt", linkInfo, nil)
		if err != nil {
			t.Fatalf("copyEntry failed: %v", err)
		}

		// Verify symlink was NOT copied (since src doesn't support ReadLink)
		if _, err := os.Lstat(filepath.Join(dstDir, "link.txt")); !errors.Is(err, fs.ErrNotExist) {
			t.Error("symlink should not have been copied when src doesn't support ReadLink")
		}
	})

	t.Run("copyEntry with symlink Symlink operation fails", func(t *testing.T) {
		// Create root directory
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		dstDir := filepath.Join(tempDir, "dst")

		// Create src and dst directories
		if err := os.Mkdir(srcDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create src dir: %v", err)
		}
		if err := os.Mkdir(dstDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create dst dir: %v", err)
		}

		// Create a symlink
		linkFile := filepath.Join(srcDir, "link.txt")
		if err := os.Symlink("target.txt", linkFile); err != nil {
			t.Fatalf("failed to create symlink: %v", err)
		}

		// Get symlink info
		linkInfo, err := os.Lstat(linkFile)
		if err != nil {
			t.Fatalf("failed to lstat symlink: %v", err)
		}

		// Create a file with the same name in dst to cause Symlink to fail
		conflictFile := filepath.Join(dstDir, "link.txt")
		if err := os.WriteFile(conflictFile, []byte("conflict"), 0o644); err != nil {
			t.Fatalf("failed to create conflict file: %v", err)
		}

		// Set up filesystems with full symlink support
		srcFs := &osfsLite{base: srcDir}
		dstFs := &osfsLite{base: dstDir}

		// Create copy option
		opt := testCopyFsOption{}

		// Copy symlink using copyEntry - should fail due to file conflict
		err = opt.copyEntry(dstFs, srcFs, "link.txt", "link.txt", linkInfo, nil)
		if err == nil {
			t.Error("expected error when symlink creation conflicts with existing file")
		}
	})
}