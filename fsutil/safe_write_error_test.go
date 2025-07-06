package fsutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/internal/osfslite"
)

// Type aliases for mock filesystem testing
type (
	testMockSafeWriteOption = SafeWriteOption[*mockErrorFs, *os.File]
)

// Test additional error paths
func TestSafeWrite_ErrorPaths(t *testing.T) {
	tempDir := t.TempDir()
	fsys := osfslite.New(tempDir)

	t.Run("Copy create error in TempFilePolicy", func(t *testing.T) {
		// Try to create a file in a non-existent directory to cause error
		nonExistentDir := filepath.Join(tempDir, "nonexistent", "nested")
		roFsys := osfslite.New(nonExistentDir)
		opt := testSafeWriteOption{}
		err := opt.Copy(roFsys, "test.txt", strings.NewReader("content"), 0o644, nil, nil)
		if err == nil {
			t.Error("expected error when creating file in non-existent directory")
		}
	})

	t.Run("CopyFs mkdir error in TempFilePolicy", func(t *testing.T) {
		// Try to create in a non-existent directory to cause error
		nonExistentDir := filepath.Join(tempDir, "nonexistent", "nested")
		roFsys := osfslite.New(nonExistentDir)
		srcFs := os.DirFS(tempDir)
		opt := testSafeWriteOption{}
		err := opt.CopyFs(roFsys, "test-dir", srcFs, 0o755, nil, nil)
		if err == nil {
			t.Error("expected error when creating directory in non-existent path")
		}
	})

	t.Run("Copy TempFilePolicyDir mkdir failure", func(t *testing.T) {
		// Try to use a TempFilePolicyDir with non-existent path
		nonExistentDir := filepath.Join(tempDir, "nonexistent", "nested")
		roFsys := osfslite.New(nonExistentDir)
		policy := newTestTempFilePolicyDir(".tmp")
		opt := testSafeWriteOption{
			TempFilePolicy: policy,
		}

		err := opt.Copy(roFsys, "test.txt", strings.NewReader("content"), 0o644, nil, nil)
		if err == nil {
			t.Error("expected error when creating temp dir in non-existent path")
		}
	})

	t.Run("Copy rename failure", func(t *testing.T) {
		// Create a directory with the target name to cause rename to fail
		targetPath := "target.txt"
		if err := os.Mkdir(filepath.Join(tempDir, targetPath), 0o755); err != nil {
			t.Fatalf("failed to create blocking directory: %v", err)
		}

		opt := testSafeWriteOption{}
		err := opt.Copy(fsys, targetPath, strings.NewReader("content"), 0o644, nil, nil)
		if err == nil {
			t.Error("expected error when rename fails")
		}
	})

	t.Run("CopyFs TempFilePolicyDir mkdir with MkdirRandom failure", func(t *testing.T) {
		// Create empty source filesystem
		srcFs := os.DirFS(tempDir)
		fsys := osfslite.New(tempDir)

		// Create temp directory first
		if err := fsys.Mkdir(".tmp", 0o755); err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}

		// Use mock filesystem that rejects mkdir operations within .tmp
		mockFs := &mockErrorFs{
			OsfsLite:       *osfslite.New(tempDir),
			mkdirError:     fs.ErrPermission,
			mkdirErrorPath: ".tmp",
		}

		mockPolicy := NewTempFilePolicyDir[*mockErrorFs](".tmp")
		opt := testMockSafeWriteOption{
			TempFilePolicy: mockPolicy,
		}

		// Try WriteFs - should fail when creating temp directory
		err := opt.CopyFs(mockFs, "target", srcFs, 0o755, nil, nil)
		if err == nil {
			t.Error("expected error when MkdirRandom fails with mock permission denied")
		}
	})

	t.Run("TempFilePolicyDir mkdir with MkdirRandom failure", func(t *testing.T) {
		// Create a new subdirectory for this test to avoid conflicts
		testSubDir := filepath.Join(tempDir, "mkdirrandom")
		if err := os.Mkdir(testSubDir, 0o755); err != nil {
			t.Fatalf("failed to create test subdir: %v", err)
		}

		// Use invalid pattern to cause MkdirRandom to fail
		policy := NewTempFilePolicyDir[*mockErrorFs](".tmp")
		fsys := osfslite.New(testSubDir)

		// Create temp directory first
		if err := fsys.Mkdir(".tmp", 0o755); err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}

		// Use mock filesystem that rejects mkdir operations
		mockFs := &mockErrorFs{
			OsfsLite:       *osfslite.New(testSubDir),
			mkdirError:     fs.ErrPermission,
			mkdirErrorPath: ".tmp",
		}

		// Try to create temp directory - should fail
		_, _, err := policy.Mkdir(mockFs, "target", 0o755)
		if err == nil {
			t.Error("expected error when MkdirRandom fails with mock permission denied")
		}
	})
}
