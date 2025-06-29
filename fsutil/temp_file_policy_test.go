package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func checkNotExist(t *testing.T, fsys fs.FS, paths ...string) {
	t.Helper()
	for _, p := range paths {
		_, err := fs.Stat(fsys, p)
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("expected errors.Is(err, %v), but got %v: %q", fs.ErrNotExist, err, p)
		}
	}
}

func TestTempFilePolicy_WalkFunc(t *testing.T) {
	t.Run("TempFilePolicyDir prefix check", func(t *testing.T) {
		tempDir := t.TempDir()
		fsys := &osfsLite{base: tempDir}

		// Create two directories: "temp" and "temp2" to verify no false prefix matches
		policy1 := newTestTempFilePolicyDir("temp")
		policy2 := newTestTempFilePolicyDir("temp2")

		// Create temp directories
		if err := fsys.Mkdir("temp", 0o755); err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		if err := fsys.Mkdir("temp2", 0o755); err != nil {
			t.Fatalf("failed to create temp2 dir: %v", err)
		}

		// Create temp files in each directory
		file1, _, err := policy1.Create(fsys, "dummy", 0o644)
		if err != nil {
			t.Fatalf("Create in temp failed: %v", err)
		}
		file1.Close()

		file2, _, err := policy2.Create(fsys, "dummy", 0o644)
		if err != nil {
			t.Fatalf("Create in temp2 failed: %v", err)
		}
		file2.Close()

		// Use WalkFunc for policy1 and ensure it only removes files from "temp", not "temp2"
		var removed []string
		var seen []string
		wrapped := &testFsysWrapper{fsys: fsys}
		err = fs.WalkDir(wrapped, ".", func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				seen = append(seen, path)
				if policy1.Match(path) {
					removed = append(removed, path)
				}
			}
			return policy1.WalkFunc(fsys, path, d, err)
		})
		if err != nil {
			t.Fatalf("WalkFunc failed: %v", err)
		}

		// Should only remove file from "temp", not "temp2"
		if len(removed) != 1 {
			t.Errorf("not equal: expected(%d) != actual(%d), seen files: %v", 1, len(removed), seen)
		}
		if len(removed) > 0 && !strings.HasPrefix(removed[0], "temp/") {
			t.Errorf("wrong file removed: %v", removed[0])
		}
	})

	t.Run("TempFilePolicyRandom", func(t *testing.T) {
		tempDir := t.TempDir()
		fsys := &osfsLite{base: tempDir}
		policy := testTempFilePolicyRandom{}

		// Create some temp files
		for range 3 {
			file, _, err := policy.Create(fsys, "dummy", 0o644)
			if err != nil {
				t.Fatalf("Create failed: %v", err)
			}
			// Verify the filename starts with "dummy." and matches the pattern
			filename := filepath.Base(file.Name())
			if !strings.HasPrefix(filename, "dummy.") {
				t.Errorf("expected filename to start with 'dummy.', got %q", filename)
			}
			if !policy.Match(filename) {
				t.Errorf("created file %q doesn't match policy pattern", filename)
			}
			file.Close()
		}

		// Create some temp directories
		for range 2 {
			dir, _, err := policy.Mkdir(fsys, "dummy", 0o755)
			if err != nil {
				t.Fatalf("Mkdir failed: %v", err)
			}
			// Verify the directory name starts with "dummy." and matches the pattern
			dirname := filepath.Base(dir.Name())
			if !strings.HasPrefix(dirname, "dummy.") {
				t.Errorf("expected dirname to start with 'dummy.', got %q", dirname)
			}
			if !policy.Match(dirname) {
				t.Errorf("created directory %q doesn't match policy pattern", dirname)
			}
			dir.Close()
		}

		// Create a non-temp file
		nonTempPath := filepath.Join(tempDir, "not-a-temp.txt")
		if err := os.WriteFile(nonTempPath, []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to create non-temp file: %v", err)
		}

		// Create a non-temp directory
		nonTempDir := filepath.Join(tempDir, "regular-dir")
		if err := os.Mkdir(nonTempDir, 0o755); err != nil {
			t.Fatalf("failed to create non-temp dir: %v", err)
		}

		// Use WalkFunc and log which files/dirs get removed
		var cleaned []string
		wrapped := &testFsysWrapper{fsys: fsys}
		err := fs.WalkDir(wrapped, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// Check if this would be processed by the policy
			if policy.Match(path) {
				cleaned = append(cleaned, path)
			}
			return policy.WalkFunc(fsys, path, d, err)
		})
		if err != nil {
			t.Fatalf("WalkFunc failed: %v", err)
		}

		// Verify temp files and directories were cleaned (3 files + 2 dirs)
		if len(cleaned) != 5 {
			t.Errorf("not equal: expected(%d) != actual(%d), cleaned: %v", 5, len(cleaned), cleaned)
		}

		checkNotExist(t, wrapped, cleaned...)

		// Verify non-temp file still exists
		if _, err := os.Stat(nonTempPath); err != nil {
			t.Error("non-temp file should still exist")
		}

		// Verify non-temp directory still exists
		if _, err := os.Stat(nonTempDir); err != nil {
			t.Error("non-temp directory should still exist")
		}
	})

	t.Run("TempFilePolicyDir", func(t *testing.T) {
		tempDir := t.TempDir()
		fsys := &osfsLite{base: tempDir}
		tempPolicyDir := ".tmp"
		policy := newTestTempFilePolicyDir(tempPolicyDir)

		// Create some temp files
		for range 3 {
			file, _, err := policy.Create(fsys, "dummy", 0o644)
			if err != nil {
				t.Fatalf("Create failed: %v", err)
			}
			file.Close()
		}

		// Create some temp directories
		for range 2 {
			dir, _, err := policy.Mkdir(fsys, "dummy", 0o755)
			if err != nil {
				t.Fatalf("Mkdir failed: %v", err)
			}
			dir.Close()
		}

		// Create a subdirectory with a temp file (should be skipped)
		subDir := filepath.Join(tempDir, tempPolicyDir, "subdir")
		if err := os.MkdirAll(subDir, 0o755); err != nil {
			t.Fatalf("failed to create subdirectory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(subDir, "0123456789.tmp"), []byte("content"), 0o644); err != nil {
			t.Fatalf("failed to create file in subdirectory: %v", err)
		}

		// Use WalkFunc and log which files/dirs get removed
		var cleaned []string
		wrapped := &testFsysWrapper{fsys: fsys}
		err := fs.WalkDir(wrapped, tempPolicyDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// Check if this would be processed by the policy
			if policy.Match(path) {
				cleaned = append(cleaned, path)
			}
			return policy.WalkFunc(fsys, path, d, err)
		})
		if err != nil {
			t.Fatalf("WalkFunc failed: %v", err)
		}

		// Verify files and directories in temp directory root were cleaned (3 files + 2 dirs, not subdirectory)
		if len(cleaned) != 5 {
			t.Errorf("not equal: expected(%d) != actual(%d), cleaned: %v", 5, len(cleaned), cleaned)
		}

		// Verify subdirectory and its file still exist
		if _, err := os.Stat(subDir); err != nil {
			t.Error("subdirectory should still exist")
		}
		if _, err := os.Stat(filepath.Join(subDir, "0123456789.tmp")); err != nil {
			t.Error("file in subdirectory should still exist")
		}
	})
}

func TestTempFilePolicy_Match(t *testing.T) {
	type testCase struct {
		name     string
		path     string
		expected bool
	}
	tests := []testCase{
		{"valid temp file with basename", "dummy.0123456789.tmp", true},
		{"valid temp file with path", "/path/to/dummy.0123456789.tmp", true},
		{"valid temp file with long basename", "verylongprefix.0123456789.tmp", true},
		{"valid temp file with complex basename", "my.file.name.0123456789.tmp", true},
		{"no dot separator", "dummy0123456789.tmp", false},
		{"too short random part", "dummy.123456789.tmp", false},
		{"too long random part", "dummy.01234567890.tmp", false},
		{"not all digits in random part", "dummy.012345678a.tmp", false},
		{"wrong extension", "dummy.0123456789.txt", false},
		{"no extension", "dummy.0123456789", false},
		{"empty", "", false},
		{"just extension", ".tmp", false},
		{"no basename", ".0123456789.tmp", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := testTempFilePolicyRandom{}
			if got := policy.Match(tt.path); got != tt.expected {
				t.Errorf("Match(%q): not equal: expected(%v) != actual(%v)", tt.path, tt.expected, got)
			}
		})
	}
}

func TestTempFilePolicy_WalkFuncErrors(t *testing.T) {
	tempDir := t.TempDir()
	fsys := &osfsLite{base: tempDir}

	t.Run("TempFilePolicyDir WalkFunc errors", func(t *testing.T) {
		policy := newTestTempFilePolicyDir(".tmp")

		// Test with walk error
		mockErr := errors.New("walk error")
		err := policy.WalkFunc(fsys, "somepath", nil, mockErr)
		if err != mockErr {
			t.Errorf("expected walk error to be returned: %v", err)
		}

		// Test with a directory that is not the temp directory
		mockDirEntry := &mockDirEntry{isDir: true}
		err = policy.WalkFunc(fsys, "other/dir", mockDirEntry, nil)
		if err != fs.SkipDir {
			t.Errorf("expected SkipDir for non-temp directory: %v", err)
		}

		// Test with exact match to temp directory
		err = policy.WalkFunc(fsys, ".tmp", mockDirEntry, nil)
		if err != nil {
			t.Errorf("expected nil error for exact temp directory: %v", err)
		}

		// Test with subdirectory inside temp directory
		err = policy.WalkFunc(fsys, filepath.Join(".tmp", "subdir"), mockDirEntry, nil)
		if err != fs.SkipDir {
			t.Errorf("expected SkipDir for subdirectory in temp dir: %v", err)
		}
	})

	t.Run("TempFilePolicyRandom Match edge cases", func(t *testing.T) {
		policy := testTempFilePolicyRandom{}

		// Test various non-matching patterns
		nonMatches := []string{
			"",
			".tmp",
			"dummy.123456789.tmp",  // too short (9 digits)
			"dummy.012345678a.tmp", // contains letter in random part
			"dummy.0123456789.txt", // wrong extension
			"dummy.0123456789",     // no extension
			"dummy0123456789.tmp",  // no dot separator
			".0123456789.tmp",      // no basename
		}

		for _, path := range nonMatches {
			if policy.Match(path) {
				t.Errorf("expected no match for %q", path)
			}
		}

		// Test patterns that should match
		matches := []string{
			"dummy.0123456789.tmp",        // basic pattern
			"a.0123456789.tmp",            // short basename
			"prefix.0123456789.tmp",       // longer basename
			"abc/dummy.0123456789.tmp",    // with path
			"abc/pre.0123456789.tmp",      // with path and basename
			"my.file.name.0123456789.tmp", // basename with dots
		}

		for _, path := range matches {
			if !policy.Match(path) {
				t.Errorf("expected match for %q", path)
			}
		}
	})
}
