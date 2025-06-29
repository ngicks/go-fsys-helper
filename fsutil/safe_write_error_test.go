package fsutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test additional error paths
func TestSafeWrite_ErrorPaths(t *testing.T) {
	tempDir := t.TempDir()
	fsys := &osfsLite{base: tempDir}

	t.Run("Copy create error in TempFilePolicy", func(t *testing.T) {
		// Create a read-only directory to cause file creation to fail
		roDir := filepath.Join(tempDir, "readonly-create")
		if err := os.Mkdir(roDir, 0o444); err != nil {
			t.Fatalf("failed to create readonly dir: %v", err)
		}
		defer os.Chmod(roDir, fs.ModePerm) // Cleanup

		roFsys := &osfsLite{base: roDir}
		opt := testSafeWriteOption{}
		err := opt.Copy(roFsys, "test.txt", strings.NewReader("content"), 0o644, nil, nil)
		if err == nil {
			t.Error("expected error when creating file in read-only directory")
		}
	})

	t.Run("CopyFs mkdir error in TempFilePolicy", func(t *testing.T) {
		// Create a read-only directory to cause directory creation to fail
		roDir := filepath.Join(tempDir, "readonly-mkdir")
		if err := os.Mkdir(roDir, 0o444); err != nil {
			t.Fatalf("failed to create readonly dir: %v", err)
		}
		defer os.Chmod(roDir, fs.ModePerm) // Cleanup

		roFsys := &osfsLite{base: roDir}
		srcFs := os.DirFS(tempDir)
		opt := testSafeWriteOption{}
		err := opt.CopyFs(roFsys, "test-dir", srcFs, 0o755, nil, nil)
		if err == nil {
			t.Error("expected error when creating directory in read-only filesystem")
		}
	})

	t.Run("Copy TempFilePolicyDir mkdir failure", func(t *testing.T) {
		// Use a read-only filesystem to cause Mkdir to fail
		roDir := filepath.Join(tempDir, "readonly-policy")
		if err := os.Mkdir(roDir, 0o444); err != nil {
			t.Fatalf("failed to create readonly dir: %v", err)
		}
		defer os.Chmod(roDir, fs.ModePerm) // Cleanup

		roFsys := &osfsLite{base: roDir}
		policy := newTestTempFilePolicyDir(".tmp")
		opt := testSafeWriteOption{
			TempFilePolicy: policy,
		}

		err := opt.Copy(roFsys, "test.txt", strings.NewReader("content"), 0o644, nil, nil)
		if err == nil {
			t.Error("expected error when creating temp dir in readonly filesystem")
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
		// Use invalid pattern to cause MkdirRandom to fail
		policy := newTestTempFilePolicyDir(".tmp")
		fsys := &osfsLite{base: tempDir}

		// Create temp directory first
		if err := fsys.Mkdir(".tmp", 0o755); err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}

		// Make temp directory read-only to cause MkdirRandom to fail
		if err := os.Chmod(filepath.Join(tempDir, ".tmp"), 0o444); err != nil {
			t.Fatalf("failed to make temp dir read-only: %v", err)
		}
		defer os.Chmod(filepath.Join(tempDir, ".tmp"), fs.ModePerm) // Cleanup

		opt := testSafeWriteOption{
			TempFilePolicy: policy,
		}

		// Try WriteFs - should fail when creating temp directory
		err := opt.CopyFs(fsys, "target", srcFs, 0o755, nil, nil)
		if err == nil {
			t.Error("expected error when MkdirRandom fails")
		}
	})

	t.Run("TempFilePolicyDir mkdir with MkdirRandom failure", func(t *testing.T) {
		// Create a new subdirectory for this test to avoid conflicts
		testSubDir := filepath.Join(tempDir, "mkdirrandom")
		if err := os.Mkdir(testSubDir, 0o755); err != nil {
			t.Fatalf("failed to create test subdir: %v", err)
		}

		// Use invalid pattern to cause MkdirRandom to fail
		policy := newTestTempFilePolicyDir(".tmp")
		fsys := &osfsLite{base: testSubDir}

		// Create temp directory first
		if err := fsys.Mkdir(".tmp", 0o755); err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}

		// Make temp directory read-only to cause MkdirRandom to fail
		if err := os.Chmod(filepath.Join(testSubDir, ".tmp"), 0o444); err != nil {
			t.Fatalf("failed to make temp dir read-only: %v", err)
		}
		defer os.Chmod(filepath.Join(testSubDir, ".tmp"), fs.ModePerm) // Cleanup

		// Try to create temp directory - should fail
		_, _, err := policy.Mkdir(fsys, "target", 0o755)
		if err == nil {
			t.Error("expected error when MkdirRandom fails")
		}
	})
}
