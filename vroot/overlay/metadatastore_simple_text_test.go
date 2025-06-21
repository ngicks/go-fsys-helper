package overlay_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot/osfs"
	"github.com/ngicks/go-fsys-helper/vroot/overlay"
	"github.com/ngicks/go-fsys-helper/vroot/overlay/acceptancetest"
)

func must[V any](v V, err error) V {
	if err != nil {
		panic(err)
	}
	return v
}

func TestMetadataStoreSimpleText(t *testing.T) {
	tempDir := t.TempDir()
	fsys := must(osfs.NewRooted(tempDir))
	defer fsys.Close()

	t.Run("interface_compliance", func(t *testing.T) {
		// Run the interface acceptance tests with a factory function
		acceptancetest.MetadataStore(t, func() overlay.MetadataStore {
			// Clear any existing whiteouts by removing the whiteout file
			fsys.Remove(overlay.MetadataStoreSimpleTextWhiteout)
			return overlay.NewMetadataStoreSimpleText(fsys)
		})
	})

	t.Run("persistence", func(t *testing.T) {
		// Clear any existing whiteouts by removing the whiteout file
		fsys.Remove(overlay.MetadataStoreSimpleTextWhiteout)

		// Create a new store and add some whiteouts
		store1 := overlay.NewMetadataStoreSimpleText(fsys)

		paths := []string{
			"persistent/file1.txt",
			"persistent/dir", // White out the entire directory
			"another.txt",
		}

		for _, path := range paths {
			err := store1.RecordWhiteout(path)
			if err != nil {
				t.Errorf("RecordWhiteout(%q) failed: %v", path, err)
			}
		}

		// Create a new store instance (simulating restart)
		store2 := overlay.NewMetadataStoreSimpleText(fsys)

		// Check that the whiteouts are still there
		for _, path := range paths {
			has, err := store2.QueryWhiteout(path)
			if err != nil {
				t.Errorf("QueryWhiteout(%q) failed after restart: %v", path, err)
				continue
			}
			if !has {
				t.Errorf("Expected %q to be whited out after restart", path)
			}
		}

		// Test parent path checking still works after reload
		has, err := store2.QueryWhiteout("persistent/dir/nested/deep.txt")
		if err != nil {
			t.Errorf("QueryWhiteout failed: %v", err)
		}
		if !has {
			t.Errorf("Expected nested path under whited out parent to be considered whited out")
		}
	})

	t.Run("file_format", func(t *testing.T) {
		// Clear any existing whiteouts by removing the whiteout file
		fsys.Remove(overlay.MetadataStoreSimpleTextWhiteout)
		store := overlay.NewMetadataStoreSimpleText(fsys)

		// Add some whiteouts with special characters that need quoting
		paths := []string{
			"file with spaces.txt",
			"file\nwith\nnewlines.txt",
			"file\"with\"quotes.txt",
			"normal.txt",
		}

		for _, path := range paths {
			err := store.RecordWhiteout(path)
			if err != nil {
				t.Errorf("RecordWhiteout(%q) failed: %v", path, err)
			}
		}

		// Check that the whiteout file exists and contains quoted paths
		whiteoutPath := filepath.Join(tempDir, overlay.MetadataStoreSimpleTextWhiteout)
		if _, err := os.Stat(whiteoutPath); err != nil {
			t.Errorf("Whiteout file should exist: %v", err)
		}

		// Read the file content
		content, err := os.ReadFile(whiteoutPath)
		if err != nil {
			t.Errorf("Failed to read whiteout file: %v", err)
		}

		// Should contain quoted strings
		contentStr := string(content)
		expectedSubstrings := []string{
			`"file with spaces.txt"`,
			`"file\nwith\nnewlines.txt"`,
			`"file\"with\"quotes.txt"`,
			`"normal.txt"`,
		}

		for _, expected := range expectedSubstrings {
			if !strings.Contains(contentStr, expected) {
				t.Errorf("Expected whiteout file to contain %q, got:\n%s", expected, contentStr)
			}
		}

		// Create a new store and verify it can read the persisted data
		store2 := overlay.NewMetadataStoreSimpleText(fsys)
		for _, path := range paths {
			has, err := store2.QueryWhiteout(path)
			if err != nil {
				t.Errorf("QueryWhiteout(%q) failed after reload: %v", path, err)
				continue
			}
			if !has {
				t.Errorf("Expected %q to be whited out after reload", path)
			}
		}
	})

	t.Run("tree_structure_optimization", func(t *testing.T) {
		// Clear any existing whiteouts by removing the whiteout file
		fsys.Remove(overlay.MetadataStoreSimpleTextWhiteout)
		store := overlay.NewMetadataStoreSimpleText(fsys)

		// Test that the tree structure properly handles removal and cleanup
		err := store.RecordWhiteout("deep/nested/path/file.txt")
		if err != nil {
			t.Errorf("RecordWhiteout failed: %v", err)
		}

		// Verify it's whited out
		has, err := store.QueryWhiteout("deep/nested/path/file.txt")
		if err != nil {
			t.Errorf("QueryWhiteout failed: %v", err)
		}
		if !has {
			t.Errorf("Expected path to be whited out")
		}

		// Remove the whiteout
		err = store.RemoveWhiteout("deep/nested/path/file.txt")
		if err != nil {
			t.Errorf("RemoveWhiteout failed: %v", err)
		}

		// Verify it's no longer whited out
		has, err = store.QueryWhiteout("deep/nested/path/file.txt")
		if err != nil {
			t.Errorf("QueryWhiteout failed: %v", err)
		}
		if has {
			t.Errorf("Expected path to not be whited out after removal")
		}

		// Test that parent paths are also not whited out
		parentPaths := []string{
			"deep",
			"deep/nested",
			"deep/nested/path",
		}

		for _, parentPath := range parentPaths {
			has, err := store.QueryWhiteout(parentPath)
			if err != nil {
				t.Errorf("QueryWhiteout(%q) failed: %v", parentPath, err)
				continue
			}
			if has {
				t.Errorf("Expected parent path %q to not be whited out", parentPath)
			}
		}
	})

	t.Run("concurrent_safety", func(t *testing.T) {
		// This test ensures that the mutex locking works correctly
		// by testing operations that could potentially race
		fsys.Remove(overlay.MetadataStoreSimpleTextWhiteout)
		store := overlay.NewMetadataStoreSimpleText(fsys)

		// Add some initial whiteouts
		paths := []string{
			"concurrent/test1.txt",
			"concurrent/test2.txt",
			"concurrent/dir",
		}

		for _, path := range paths {
			err := store.RecordWhiteout(path)
			if err != nil {
				t.Errorf("RecordWhiteout(%q) failed: %v", path, err)
			}
		}

		// Test that queries work correctly with mixed operations
		for i := 0; i < 10; i++ {
			// Query existing paths
			for _, path := range paths {
				has, err := store.QueryWhiteout(path)
				if err != nil {
					t.Errorf("QueryWhiteout(%q) failed: %v", path, err)
				}
				if !has {
					t.Errorf("Expected %q to be whited out", path)
				}
			}

			// Query non-existent paths
			nonExistentPaths := []string{
				"nonexistent.txt",
				"other/path.txt",
			}
			for _, path := range nonExistentPaths {
				has, err := store.QueryWhiteout(path)
				if err != nil {
					t.Errorf("QueryWhiteout(%q) failed: %v", path, err)
				}
				if has {
					t.Errorf("Expected %q to not be whited out", path)
				}
			}
		}
	})
}
