package overlayfs

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestOverlay_Rename(t *testing.T) {
	type testCase struct {
		name func() string
		from string
		to   string
		err  error
	}

	cases := []testCase{
		{
			name: func() string {
				return "rename file to dir at under layers"
			},
			from: "root/readable/file1.txt",
			to:   "root/readable/subdir",
			err:  fs.ErrExist,
		},
		{
			name: func() string {
				return "rename dir to file at under layers"
			},
			from: "root/readable/subdir",
			to:   "root/readable/file1.txt",
			err:  fs.ErrExist,
		},
		{
			name: func() string {
				return "create and rename new file successfully"
			},
			from: "root/writable/new_file.txt",
			to:   "root/writable/renamed_file.txt",
			err:  nil,
		},
		{
			name: func() string {
				return "create and rename new directory successfully"
			},
			from: "root/writable/new_dir/",
			to:   "root/writable/renamed_dir/",
			err:  nil,
		},
		{
			name: func() string {
				return "create nested directory and rename"
			},
			from: "root/writable/deep/nested/dir/",
			to:   "root/writable/moved_nested_dir/",
			err:  nil,
		},
		{
			name: func() string {
				return "create file in nested path and rename"
			},
			from: "root/writable/nested/path/file.log",
			to:   "root/writable/renamed.log",
			err:  nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name(), func(t *testing.T) {
			tempDir := t.TempDir()
			t.Logf("temp dir = %s", tempDir)

			r, closers := prepareLayers(tempDir)
			defer r.Close()
			defer closers(t)

			// Create files/directories for new test cases based on path pattern
			// Use r.top directly for more accurate testing of overlay logic
			if strings.HasSuffix(tc.from, "/") {
				// Path ends with slash - create directory in top layer
				if err := r.top.MkdirAll(tc.from, fs.ModePerm); err != nil {
					t.Fatalf("failed to create directory %s in top layer: %v", tc.from, err)
				}
			} else if !strings.Contains(tc.from, "root/readable/") {
				// Path doesn't end with slash and is not in read-only area - create file in top layer
				// First ensure parent directory exists in top layer
				if err := r.top.MkdirAll(filepath.Dir(tc.from), fs.ModePerm); err != nil {
					t.Fatalf("failed to create parent directory for %s in top layer: %v", tc.from, err)
				}
				// Create new file in top layer
				f, err := r.top.Create(tc.from)
				if err != nil {
					t.Fatalf("failed to create file %s in top layer: %v", tc.from, err)
				}
				defer f.Close()
				if _, err := f.Write([]byte("test content")); err != nil {
					t.Fatalf("failed to write to file %s in top layer: %v", tc.from, err)
				}
			}

			err := r.Rename(tc.from, tc.to)
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Errorf("expected error %v, got %v", tc.err, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				// For successful renames, verify the file/directory exists at new location
				if _, statErr := r.Lstat(tc.to); statErr != nil {
					t.Errorf("renamed file/directory not found at %s: %v", tc.to, statErr)
				}
				// And verify it doesn't exist at old location
				if _, statErr := r.Lstat(tc.from); !errors.Is(statErr, fs.ErrNotExist) {
					t.Errorf("expected file/directory to be gone from %s, but got error: %v", tc.from, statErr)
				}
			}
		})
	}
}
