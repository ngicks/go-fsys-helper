package overlayfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func TestCopyPolicyDotTmp_AllTypes(t *testing.T) {
	tempDir := t.TempDir()

	// Create source and destination directories
	srcDir := filepath.Join(tempDir, "src")
	dstDir := filepath.Join(tempDir, "dst")

	err := os.MkdirAll(srcDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create source directory: %v", err)
	}

	err = os.MkdirAll(dstDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create destination directory: %v", err)
	}

	// Create test files
	testFile := filepath.Join(srcDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create test directory
	testDir := filepath.Join(srcDir, "testdir")
	err = os.Mkdir(testDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create test symlink
	testSymlink := filepath.Join(srcDir, "testsymlink")
	err = os.Symlink("test.txt", testSymlink)
	if err != nil {
		t.Fatalf("Failed to create test symlink: %v", err)
	}

	// Create filesystem wrappers
	srcFs, err := osfs.NewRooted(srcDir)
	if err != nil {
		t.Fatalf("Failed to create source filesystem: %v", err)
	}
	defer srcFs.Close()

	dstFs, err := osfs.NewRooted(dstDir)
	if err != nil {
		t.Fatalf("Failed to create destination filesystem: %v", err)
	}
	defer dstFs.Close()

	// Create copy policy
	copyPolicy := NewCopyPolicyDotTmp("*.tmp")

	// Create layer
	layer := Layer{
		meta: &simpleMetadataStore{},
		fsys: srcFs,
	}

	// Test copying regular file
	t.Run("copy file", func(t *testing.T) {
		err := copyPolicy.CopyTo(layer, dstFs, "test.txt")
		if err != nil {
			t.Fatalf("Failed to copy file: %v", err)
		}

		// Verify file exists and has correct content
		content, err := os.ReadFile(filepath.Join(dstDir, "test.txt"))
		if err != nil {
			t.Fatalf("Failed to read copied file: %v", err)
		}

		if string(content) != "test content" {
			t.Errorf("File content mismatch: got %q, want %q", string(content), "test content")
		}
	})

	// Test copying directory
	t.Run("copy directory", func(t *testing.T) {
		err := copyPolicy.CopyTo(layer, dstFs, "testdir")
		if err != nil {
			t.Fatalf("Failed to copy directory: %v", err)
		}

		// Verify directory exists
		info, err := os.Stat(filepath.Join(dstDir, "testdir"))
		if err != nil {
			t.Fatalf("Failed to stat copied directory: %v", err)
		}

		if !info.IsDir() {
			t.Errorf("Copied item is not a directory")
		}
	})

	// Test copying symlink
	t.Run("copy symlink", func(t *testing.T) {
		err := copyPolicy.CopyTo(layer, dstFs, "testsymlink")
		if err != nil {
			t.Fatalf("Failed to copy symlink: %v", err)
		}

		// Verify symlink exists and has correct target
		info, err := os.Lstat(filepath.Join(dstDir, "testsymlink"))
		if err != nil {
			t.Fatalf("Failed to lstat copied symlink: %v", err)
		}

		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("Copied item is not a symlink")
		}

		target, err := os.Readlink(filepath.Join(dstDir, "testsymlink"))
		if err != nil {
			t.Fatalf("Failed to readlink copied symlink: %v", err)
		}

		if target != "test.txt" {
			t.Errorf("Symlink target mismatch: got %q, want %q", target, "test.txt")
		}
	})
}

// Simple metadata store for testing
type simpleMetadataStore struct{}

func (s *simpleMetadataStore) QueryWhiteout(name string) (bool, error) {
	return false, nil
}

func (s *simpleMetadataStore) RecordWhiteout(name string) error {
	return nil
}

func (s *simpleMetadataStore) RemoveWhiteout(name string) error {
	return nil
}
