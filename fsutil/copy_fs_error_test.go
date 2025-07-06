package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/internal/osfslite"
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

		// Create a directory that will be mocked as unreadable
		unreadableDir := filepath.Join(srcDir, "unreadable")
		if err := os.Mkdir(unreadableDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create unreadable dir: %v", err)
		}

		srcFs := &mockErrorDirFs{base: os.DirFS(srcDir)}
		dstFs := osfslite.New(dstDir)

		opt := testCopyFsOption{}

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
		dstFs := osfslite.New(dstDir)

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
		if err := os.Mkdir(dstDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create dst dir: %v", err)
		}

		// Create source file in subdirectory
		if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), fs.ModePerm); err != nil {
			t.Fatalf("failed to create source subdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "subdir", "file.txt"), []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to create source file: %v", err)
		}

		// Set up filesystems with mock mkdir error
		srcFs := os.DirFS(srcDir)
		dstFs := &mockErrorFs{
			OsfsLite:       *osfslite.New(dstDir),
			mkdirError:     fs.ErrPermission,
			mkdirErrorPath: "subdir",
		}

		// Create copy option
		opt := testMockCopyFsOption{}

		// Try to copy - should fail when creating directory
		err := opt.CopyPath(dstFs, srcFs, ".", filepath.FromSlash("subdir/file.txt"))
		if err == nil {
			t.Error("expected error when creating directory in mock read-only filesystem")
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
		err := opt.copyEntry(osfslite.New(dstDir), os.DirFS(srcDir), "path", "path", nil, mockErr)
		if err != mockErr {
			t.Errorf("expected walk error to be returned")
		}

		// Create a file that we'll mock as unreadable
		unreadableFile := filepath.Join(srcDir, "unreadable.txt")
		if err := os.WriteFile(unreadableFile, []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to create unreadable file: %v", err)
		}

		// Try to copy unreadable file using mock error fs
		info, _ := os.Stat(unreadableFile)
		mockSrcFs := &mockErrorSrcFs{
			base:      os.DirFS(srcDir),
			openError: fs.ErrPermission,
			openPath:  "unreadable.txt",
		}
		err = opt.copyEntry(osfslite.New(dstDir), mockSrcFs, "unreadable.txt", "unreadable.txt", info, nil)
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
		srcFs := os.DirFS(srcDir)
		dstFs := osfslite.New(dstDir)

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

		// Set up filesystems with mock Lstat error
		srcFs := &mockLstatFs{
			base:       os.DirFS(srcDir),
			lstatError: fs.ErrPermission,
			lstatPath:  "broken",
		}
		dstFs := osfslite.New(dstDir)

		// Create copy option
		opt := testCopyFsOption{}

		// Perform copy - should fail due to Lstat permission issues
		err := opt.CopyAll(dstFs, srcFs, ".")
		if err == nil {
			t.Error("expected error when copying with Lstat permission issues")
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

		// Set up filesystems where src doesn't support ReadLink (BasicWrapper doesn't implement ReadLinkFs)
		srcFs := osfslite.NewBasicWrapper(srcDir)
		dstFs := osfslite.New(dstDir)

		// Create copy option
		opt := testCopyFsOption{}

		// Copy symlink using copyEntry - should ignore the symlink
		err = opt.copyEntry(dstFs, srcFs, "link.txt", "link.txt", linkInfo, nil)
		if err != nil {
			t.Fatalf("copyEntry failed: %v", err)
		}

		// Verify symlink was NOT copied (since src doesn't support ReadLink)
		if _, err := os.Lstat(filepath.Join(dstDir, "link.txt")); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("symlink should not have been copied when src doesn't support ReadLink: %v", err)
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

		// Set up filesystems with full symlink support (os.DirFS now supports ReadLink in Go 1.25)
		srcFs := os.DirFS(srcDir)

		// Create copy option
		opt := testMockCopyFsOption{}

		// Set up mock filesystem that will fail on symlink creation
		mockDstFs := &mockErrorFs{
			OsfsLite:           *osfslite.New(dstDir),
			symlinkError:       fs.ErrExist,
			symlinkErrorTarget: "link.txt",
		}

		// Copy symlink using copyEntry - should fail due to mock symlink error
		err = opt.copyEntry(mockDstFs, srcFs, "link.txt", "link.txt", linkInfo, nil)
		if err == nil {
			t.Error("expected error when symlink creation conflicts with existing file")
		}
	})

	t.Run("CopyAll with IgnoreErr for walk errors", func(t *testing.T) {
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

		// Create a directory that will be mocked as unreadable during walk
		unreadableDir := filepath.Join(srcDir, "unreadable")
		if err := os.Mkdir(unreadableDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create unreadable dir: %v", err)
		}

		// Create a readable file to verify partial success
		readableFile := filepath.Join(srcDir, "readable.txt")
		if err := os.WriteFile(readableFile, []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to create readable file: %v", err)
		}

		srcFs := &mockErrorDirFs{base: os.DirFS(srcDir)}
		dstFs := osfslite.New(dstDir)

		opt := CopyFsOption[*osfslite.OsfsLite, *os.File]{
			IgnoreErr: func(err error) bool {
				return errors.Is(err, fs.ErrPermission)
			},
		}

		err := opt.CopyAll(dstFs, srcFs, ".")
		if err != nil {
			t.Errorf("expected no error when ignoring walk permission errors, got: %v", err)
		}

		// Verify that readable file was copied
		copiedContent, err := os.ReadFile(filepath.Join(dstDir, "readable.txt"))
		if err != nil {
			t.Errorf("failed to read copied file: %v", err)
		}
		if string(copiedContent) != "content" {
			t.Errorf("copied file content mismatch: expected %q, got %q", "content", string(copiedContent))
		}
	})

	t.Run("CopyAll IgnoreErr filter specific walk errors", func(t *testing.T) {
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

		// Create a directory that will be mocked as unreadable
		unreadableDir := filepath.Join(srcDir, "unreadable")
		if err := os.Mkdir(unreadableDir, fs.ModePerm); err != nil {
			t.Fatalf("failed to create unreadable dir: %v", err)
		}

		srcFs := &mockErrorDirFs{base: os.DirFS(srcDir)}
		dstFs := osfslite.New(dstDir)

		// Test with IgnoreErr that doesn't match the error
		opt := CopyFsOption[*osfslite.OsfsLite, *os.File]{
			IgnoreErr: func(err error) bool {
				return errors.Is(err, fs.ErrNotExist) // Only ignore NotExist errors
			},
		}

		err := opt.CopyAll(dstFs, srcFs, ".")
		if err == nil {
			t.Error("expected error when not ignoring permission errors")
		}
		if !errors.Is(err, fs.ErrPermission) {
			t.Errorf("expected permission error, got: %v", err)
		}
	})
}
