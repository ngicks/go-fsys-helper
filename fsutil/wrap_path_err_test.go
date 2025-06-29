package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"testing"
)

func TestWrapPathErr(t *testing.T) {
	type testCase struct {
		name      string
		op        string
		path      string
		err       error
		expected  string
		isPathErr bool
	}
	tests := []testCase{
		{
			name:      "nil error",
			op:        "open",
			path:      "/test/path",
			err:       nil,
			expected:  "",
			isPathErr: false,
		},
		{
			name:      "path error",
			op:        "open",
			path:      "/test/path",
			err:       &fs.PathError{Op: "open", Path: "/test/path", Err: fs.ErrNotExist},
			expected:  "open /test/path",
			isPathErr: true,
		},
		{
			name:      "non-path error",
			op:        "read",
			path:      "/test/file",
			err:       errors.New("some error"),
			expected:  "read /test/file: some error",
			isPathErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapPathErr(tt.op, tt.path, tt.err)

			if tt.err == nil {
				if result != nil {
					t.Errorf("expected nil error, got %v", result)
				}
				return
			}

			if tt.isPathErr {
				var pathErr *fs.PathError
				if !errors.As(result, &pathErr) {
					t.Errorf("expected PathError, got %T", result)
				} else {
					if pathErr.Op != tt.op {
						t.Errorf("op mismatch: expected %q, got %q", tt.op, pathErr.Op)
					}
					if pathErr.Path != tt.path {
						t.Errorf("path mismatch: expected %q, got %q", tt.path, pathErr.Path)
					}
				}
			}
		})
	}
}

func TestWrapLinkErr(t *testing.T) {
	type testCase struct {
		name      string
		op        string
		old       string
		new       string
		err       error
		expected  string
		isLinkErr bool
	}
	tests := []testCase{
		{
			name:      "nil error",
			op:        "symlink",
			old:       "/old/path",
			new:       "/new/path",
			err:       nil,
			expected:  "",
			isLinkErr: false,
		},
		{
			name:      "link error",
			op:        "symlink",
			old:       "/old/path",
			new:       "/new/path",
			err:       &os.LinkError{Op: "symlink", Old: "/old/path", New: "/new/path", Err: fs.ErrExist},
			expected:  "symlink /old/path /new/path",
			isLinkErr: true,
		},
		{
			name:      "non-link error",
			op:        "link",
			old:       "/source",
			new:       "/target",
			err:       errors.New("permission denied"),
			expected:  "link /source /target: permission denied",
			isLinkErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WrapLinkErr(tt.op, tt.old, tt.new, tt.err)

			if tt.err == nil {
				if result != nil {
					t.Errorf("expected nil error, got %v", result)
				}
				return
			}

			if tt.isLinkErr {
				var linkErr *os.LinkError
				if !errors.As(result, &linkErr) {
					t.Errorf("expected LinkError, got %T", result)
				} else {
					if linkErr.Op != tt.op {
						t.Errorf("op mismatch: expected %q, got %q", tt.op, linkErr.Op)
					}
					if linkErr.Old != tt.old {
						t.Errorf("old mismatch: expected %q, got %q", tt.old, linkErr.Old)
					}
					if linkErr.New != tt.new {
						t.Errorf("new mismatch: expected %q, got %q", tt.new, linkErr.New)
					}
				}
			}
		})
	}
}
