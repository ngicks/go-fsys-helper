package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/internal/osfslite"
	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
)

type testCopyFsOption = CopyFsOption[*osfslite.OsfsLite, *os.File]

func TestCopyFs(t *testing.T) {
	t.Run("basic copy", func(t *testing.T) {
		// Create root directory
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		dstDir := filepath.Join(tempDir, "dst")

		// Create test structure using testhelper
		err := testhelper.ExecuteLines(tempDir,
			"src/",
			"dst/",
			"src/subdir/",
			"src/file1.txt: 0644 content1",
			"src/subdir/file2.txt: 0755 content2",
		)
		if err != nil {
			t.Fatalf("failed to create test structure: %v", err)
		}

		// Set up filesystems
		srcFs := os.DirFS(srcDir)
		dstFs := osfslite.New(dstDir)

		// Create copy option
		opt := testCopyFsOption{}

		// Perform copy
		err = opt.CopyAll(dstFs, srcFs, ".")
		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}

		// Verify files were copied
		data1, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
		if err != nil {
			t.Fatalf("failed to read copied file1: %v", err)
		}
		if string(data1) != "content1" {
			t.Errorf("not equal: expected(%q) != actual(%q)", "content1", string(data1))
		}

		data2, err := os.ReadFile(filepath.Join(dstDir, "subdir", "file2.txt"))
		if err != nil {
			t.Fatalf("failed to read copied file2: %v", err)
		}
		if string(data2) != "content2" {
			t.Errorf("not equal: expected(%q) != actual(%q)", "content2", string(data2))
		}

		// Verify directory exists
		info, err := os.Stat(filepath.Join(dstDir, "subdir"))
		if err != nil {
			t.Fatalf("copied subdir does not exist: %v", err)
		}
		if !info.IsDir() {
			t.Error("copied subdir is not a directory")
		}

		// Verify permissions were preserved from source
		info1, err := os.Stat(filepath.Join(dstDir, "file1.txt"))
		if err != nil {
			t.Fatalf("failed to stat copied file1: %v", err)
		}
		expectedPerm1 := fs.FileMode(0o644)
		if runtime.GOOS == "windows" {
			expectedPerm1 = 0o666 // Windows typically widens files to read-write
		}
		if info1.Mode().Perm() != expectedPerm1 {
			t.Errorf("file1 permissions: not equal: expected(%o) != actual(%o)", expectedPerm1, info1.Mode().Perm())
		}

		info2, err := os.Stat(filepath.Join(dstDir, "subdir", "file2.txt"))
		if err != nil {
			t.Fatalf("failed to stat copied file2: %v", err)
		}
		expectedPerm2 := fs.FileMode(0o755)
		if runtime.GOOS == "windows" {
			expectedPerm2 = 0o666 // Windows typically widens files to read-write (0o755 is file perm, not dir)
		}
		if info2.Mode().Perm() != expectedPerm2 {
			t.Errorf("file2 permissions: not equal: expected(%o) != actual(%o)", expectedPerm2, info2.Mode().Perm())
		}
	})

	t.Run("preserved permissions", func(t *testing.T) {
		// Create root directory
		tempDir := t.TempDir()
		srcDir := filepath.Join(tempDir, "src")
		dstDir := filepath.Join(tempDir, "dst")

		// Create test structure using testhelper
		err := testhelper.ExecuteLines(tempDir,
			"src/",
			"dst/",
			"src/file.txt: 0600 content",
		)
		if err != nil {
			t.Fatalf("failed to create test structure: %v", err)
		}

		// Set up filesystems
		srcFs := os.DirFS(srcDir)
		dstFs := osfslite.New(dstDir)

		// Create copy option
		opt := testCopyFsOption{}

		// Perform copy
		err = opt.CopyAll(dstFs, srcFs, ".")
		if err != nil {
			t.Fatalf("Copy failed: %v", err)
		}

		// Verify file permissions were preserved from source (0o600)
		info, err := os.Stat(filepath.Join(dstDir, "file.txt"))
		if err != nil {
			t.Fatalf("failed to stat copied file: %v", err)
		}
		expectedPerm := fs.FileMode(0o600)
		if runtime.GOOS == "windows" {
			expectedPerm = 0o666 // Windows typically widens files to read-write
		}
		if info.Mode().Perm() != expectedPerm {
			t.Errorf("file permissions: not equal: expected(%o) != actual(%o)", expectedPerm, info.Mode().Perm())
		}
	})
}

func TestCopyPath(t *testing.T) {
	t.Run("copy specific files", func(t *testing.T) {
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

		// Create source files and directories
		if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), fs.ModePerm); err != nil {
			t.Fatalf("failed to create source subdir: %v", err)
		}

		if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0o644); err != nil {
			t.Fatalf("failed to create source file1: %v", err)
		}

		if err := os.WriteFile(filepath.Join(srcDir, "file2.txt"), []byte("content2"), fs.ModePerm); err != nil {
			t.Fatalf("failed to create source file2: %v", err)
		}

		if err := os.WriteFile(filepath.Join(srcDir, "subdir", "file3.txt"), []byte("content3"), 0o644); err != nil {
			t.Fatalf("failed to create source file3: %v", err)
		}

		// Set up filesystems
		srcFs := os.DirFS(srcDir)
		dstFs := osfslite.New(dstDir)

		// Create copy option
		opt := testCopyFsOption{}

		// Copy only specific files
		err := opt.CopyPath(dstFs, srcFs, ".", "file1.txt", filepath.FromSlash("subdir/file3.txt"))
		if err != nil {
			t.Fatalf("CopyPath failed: %v", err)
		}

		// Verify file1.txt was copied
		data1, err := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
		if err != nil {
			t.Fatalf("failed to read copied file1: %v", err)
		}
		if string(data1) != "content1" {
			t.Errorf("not equal: expected(%q) != actual(%q)", "content1", string(data1))
		}

		// Verify subdir/file3.txt was copied (with directory creation)
		data3, err := os.ReadFile(filepath.Join(dstDir, "subdir", "file3.txt"))
		if err != nil {
			t.Fatalf("failed to read copied file3: %v", err)
		}
		if string(data3) != "content3" {
			t.Errorf("not equal: expected(%q) != actual(%q)", "content3", string(data3))
		}

		// Verify file2.txt was NOT copied
		if _, err := os.Stat(filepath.Join(dstDir, "file2.txt")); !errors.Is(err, fs.ErrNotExist) {
			t.Error("file2.txt should not have been copied")
		}
	})

	t.Run("copy directory", func(t *testing.T) {
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

		// Create source directory
		if err := os.Mkdir(filepath.Join(srcDir, "testdir"), fs.ModePerm); err != nil {
			t.Fatalf("failed to create source dir: %v", err)
		}

		// Set up filesystems
		srcFs := os.DirFS(srcDir)
		dstFs := osfslite.New(dstDir)

		// Create copy option
		opt := testCopyFsOption{}

		// Copy directory
		err := opt.CopyPath(dstFs, srcFs, ".", "testdir")
		if err != nil {
			t.Fatalf("CopyPath failed: %v", err)
		}

		// Verify directory was created
		info, err := os.Stat(filepath.Join(dstDir, "testdir"))
		if err != nil {
			t.Fatalf("copied directory does not exist: %v", err)
		}
		if !info.IsDir() {
			t.Error("copied path is not a directory")
		}
	})
}
