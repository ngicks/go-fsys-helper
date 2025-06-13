package acceptancetest

import (
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot/overlay"
)

// MetadataStore runs acceptance tests for any MetadataStore implementation.
// It tests the interface contract without depending on implementation details.
// The store should be clean/empty when passed to this function.
func MetadataStore(t *testing.T, newStore func() overlay.MetadataStore) {
	t.Run("basic_operations", func(t *testing.T) {
		store := newStore()
		
		// Test recording a whiteout
		err := store.RecordWhiteout("file.txt")
		if err != nil {
			t.Errorf("RecordWhiteout failed: %v", err)
		}

		// Test querying the whiteout
		has, err := store.QueryWhiteout("file.txt")
		if err != nil {
			t.Errorf("QueryWhiteout failed: %v", err)
		}
		if !has {
			t.Errorf("Expected file.txt to be whited out")
		}

		// Test removing the whiteout
		err = store.RemoveWhiteout("file.txt")
		if err != nil {
			t.Errorf("RemoveWhiteout failed: %v", err)
		}

		// Test querying after removal
		has, err = store.QueryWhiteout("file.txt")
		if err != nil {
			t.Errorf("QueryWhiteout after removal failed: %v", err)
		}
		if has {
			t.Errorf("Expected file.txt to not be whited out after removal")
		}
	})

	t.Run("parent_path_checking", func(t *testing.T) {
		store := newStore()
		
		// Whiteout a parent directory
		err := store.RecordWhiteout("dir")
		if err != nil {
			t.Errorf("RecordWhiteout failed: %v", err)
		}

		// Check that child paths are also considered whited out
		type testCase struct {
			path     string
			expected bool
		}
		testCases := []testCase{
			{"dir", true},                 // The whited out path itself
			{"dir/file.txt", true},        // Direct child
			{"dir/subdir", true},          // Direct child directory
			{"dir/subdir/file.txt", true}, // Nested child
			{"dir/a/b/c/d/e.txt", true},   // Deeply nested child
			{"other.txt", false},          // Unrelated file
			{"directory", false},          // Similar name but different
			{"dir2", false},               // Similar name but different
		}

		for _, tc := range testCases {
			has, err := store.QueryWhiteout(tc.path)
			if err != nil {
				t.Errorf("QueryWhiteout(%q) failed: %v", tc.path, err)
				continue
			}
			if has != tc.expected {
				t.Errorf("QueryWhiteout(%q) = %v, expected %v", tc.path, has, tc.expected)
			}
		}
	})

	t.Run("nested_whiteouts", func(t *testing.T) {
		store := newStore()
		
		// Record nested whiteouts
		paths := []string{
			"a/b/c/file1.txt",
			"a/b/file2.txt",
			"a/file3.txt",
			"x/y/z/deep.txt",
		}

		for _, path := range paths {
			err := store.RecordWhiteout(path)
			if err != nil {
				t.Errorf("RecordWhiteout(%q) failed: %v", path, err)
			}
		}

		// Test that all recorded paths are whited out
		for _, path := range paths {
			has, err := store.QueryWhiteout(path)
			if err != nil {
				t.Errorf("QueryWhiteout(%q) failed: %v", path, err)
				continue
			}
			if !has {
				t.Errorf("Expected %q to be whited out", path)
			}
		}

		// Test that parents are NOT whited out (unless explicitly set)
		type parentTestCase struct {
			path     string
			expected bool
		}
		parentTests := []parentTestCase{
			{"a", false},     // Parent of a/file3.txt
			{"a/b", false},   // Parent of a/b/file2.txt
			{"a/b/c", false}, // Parent of a/b/c/file1.txt
			{"x", false},     // Parent of x/y/z/deep.txt
			{"x/y", false},   // Parent of x/y/z/deep.txt
			{"x/y/z", false}, // Parent of x/y/z/deep.txt
		}

		for _, tc := range parentTests {
			has, err := store.QueryWhiteout(tc.path)
			if err != nil {
				t.Errorf("QueryWhiteout(%q) failed: %v", tc.path, err)
				continue
			}
			if has != tc.expected {
				t.Errorf("QueryWhiteout(%q) = %v, expected %v", tc.path, has, tc.expected)
			}
		}
	})

	t.Run("root_whiteout_rejected", func(t *testing.T) {
		store := newStore()
		
		// Test that whiting out the root is rejected
		err := store.RecordWhiteout(".")
		if err == nil {
			t.Errorf("RecordWhiteout('.') should have failed but didn't")
		}

		// Various forms of root path should all be rejected
		rootPaths := []string{
			".",
			"",
			"./",
			"./.",
		}

		for _, rootPath := range rootPaths {
			err := store.RecordWhiteout(rootPath)
			if err == nil {
				t.Errorf("RecordWhiteout(%q) should have failed but didn't", rootPath)
			}
		}

		// Query for root should return false since it can't be whited out
		has, err := store.QueryWhiteout(".")
		if err != nil {
			t.Errorf("QueryWhiteout('.') failed: %v", err)
		}
		if has {
			t.Errorf("Expected '.' to not be whited out since it cannot be recorded")
		}
	})

	t.Run("remove_with_children", func(t *testing.T) {
		store := newStore()
		
		// Add a parent and child whiteout
		err := store.RecordWhiteout("parent")
		if err != nil {
			t.Errorf("RecordWhiteout failed: %v", err)
		}
		err = store.RecordWhiteout("parent/child.txt")
		if err != nil {
			t.Errorf("RecordWhiteout failed: %v", err)
		}

		// Remove the parent whiteout
		err = store.RemoveWhiteout("parent")
		if err != nil {
			t.Errorf("RemoveWhiteout failed: %v", err)
		}

		// Parent should no longer be whited out
		has, err := store.QueryWhiteout("parent")
		if err != nil {
			t.Errorf("QueryWhiteout failed: %v", err)
		}
		if has {
			t.Errorf("Expected parent to not be whited out after removal")
		}

		// But child should still be whited out
		has, err = store.QueryWhiteout("parent/child.txt")
		if err != nil {
			t.Errorf("QueryWhiteout failed: %v", err)
		}
		if !has {
			t.Errorf("Expected child to still be whited out")
		}

		// Other children should not be whited out due to parent removal
		has, err = store.QueryWhiteout("parent/other.txt")
		if err != nil {
			t.Errorf("QueryWhiteout failed: %v", err)
		}
		if has {
			t.Errorf("Expected other child to not be whited out after parent removal")
		}
	})

	t.Run("edge_cases", func(t *testing.T) {
		store := newStore()
		
		// Test empty paths and edge cases
		type edgeTestCase struct {
			record string
			query  string
			expect bool
		}
		testCases := []edgeTestCase{
			{"a", "a", true},
			{"a", "a/", true}, // Trailing slash should be handled
			{"a/", "a", true}, // Trailing slash in record should be handled
			{"a/b", "a/b/", true},
		}

		for _, tc := range testCases {
			err := store.RecordWhiteout(tc.record)
			if err != nil {
				t.Errorf("RecordWhiteout(%q) failed: %v", tc.record, err)
				continue
			}

			has, err := store.QueryWhiteout(tc.query)
			if err != nil {
				t.Errorf("QueryWhiteout(%q) failed: %v", tc.query, err)
				continue
			}
			if has != tc.expect {
				t.Errorf("QueryWhiteout(%q) after RecordWhiteout(%q) = %v, expected %v",
					tc.query, tc.record, has, tc.expect)
			}
		}
	})
}