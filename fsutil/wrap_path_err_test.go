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

// TestWrapPathErr_doesNotMutateInput asserts the copy-on-write behavior: when
// the input is already a *fs.PathError, the caller's value must be left
// untouched and a fresh copy returned (see plan 01 entry F1).
func TestWrapPathErr_doesNotMutateInput(t *testing.T) {
	input := &fs.PathError{Op: "origOp", Path: "/orig/path", Err: fs.ErrNotExist}
	orig := *input

	result := WrapPathErr("newOp", "/new/path", input)

	// Input must be unchanged.
	if *input != orig {
		t.Errorf("input was mutated: got %+v, want %+v", *input, orig)
	}

	// The returned value must be a distinct allocation with the merged fields.
	var resultErr *fs.PathError
	if !errors.As(result, &resultErr) {
		t.Fatalf("expected *fs.PathError, got %T", result)
	}
	if resultErr == input {
		t.Error("expected a fresh copy, got the same pointer as input")
	}
	if resultErr.Op != "newOp" {
		t.Errorf("op mismatch: got %q, want %q", resultErr.Op, "newOp")
	}
	if resultErr.Path != "/new/path" {
		t.Errorf("path mismatch: got %q, want %q", resultErr.Path, "/new/path")
	}
	if resultErr.Err != input.Err {
		t.Errorf("wrapped err mismatch: got %v, want %v", resultErr.Err, input.Err)
	}
}

// TestWrapPathErr_emptyFieldsPreserveOriginal asserts empty op/path leave the
// copy's corresponding fields untouched while still not mutating the input.
func TestWrapPathErr_emptyFieldsPreserveOriginal(t *testing.T) {
	input := &fs.PathError{Op: "origOp", Path: "/orig/path", Err: fs.ErrNotExist}
	orig := *input

	result := WrapPathErr("", "", input)

	if *input != orig {
		t.Errorf("input was mutated: got %+v, want %+v", *input, orig)
	}
	var resultErr *fs.PathError
	if !errors.As(result, &resultErr) {
		t.Fatalf("expected *fs.PathError, got %T", result)
	}
	if resultErr.Op != "origOp" || resultErr.Path != "/orig/path" {
		t.Errorf("empty op/path should preserve original fields, got %+v", *resultErr)
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
			name: "link error",
			op:   "symlink",
			old:  "/old/path",
			new:  "/new/path",
			err: &os.LinkError{
				Op:  "symlink",
				Old: "/old/path",
				New: "/new/path",
				Err: fs.ErrExist,
			},
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

// TestWrapLinkErr_doesNotMutateInput asserts the copy-on-write behavior for
// *os.LinkError inputs (see plan 01 entry F1).
func TestWrapLinkErr_doesNotMutateInput(t *testing.T) {
	input := &os.LinkError{Op: "origOp", Old: "/orig/old", New: "/orig/new", Err: fs.ErrExist}
	orig := *input

	result := WrapLinkErr("newOp", "/new/old", "/new/new", input)

	if *input != orig {
		t.Errorf("input was mutated: got %+v, want %+v", *input, orig)
	}

	var resultErr *os.LinkError
	if !errors.As(result, &resultErr) {
		t.Fatalf("expected *os.LinkError, got %T", result)
	}
	if resultErr == input {
		t.Error("expected a fresh copy, got the same pointer as input")
	}
	if resultErr.Op != "newOp" || resultErr.Old != "/new/old" || resultErr.New != "/new/new" {
		t.Errorf("merged fields mismatch, got %+v", *resultErr)
	}
	if resultErr.Err != input.Err {
		t.Errorf("wrapped err mismatch: got %v, want %v", resultErr.Err, input.Err)
	}
}

// TestWrapLinkErr_emptyFieldsPreserveOriginal asserts empty op/old/new leave
// the copy's corresponding fields untouched without mutating the input.
func TestWrapLinkErr_emptyFieldsPreserveOriginal(t *testing.T) {
	input := &os.LinkError{Op: "origOp", Old: "/orig/old", New: "/orig/new", Err: fs.ErrExist}
	orig := *input

	result := WrapLinkErr("", "", "", input)

	if *input != orig {
		t.Errorf("input was mutated: got %+v, want %+v", *input, orig)
	}
	var resultErr *os.LinkError
	if !errors.As(result, &resultErr) {
		t.Fatalf("expected *os.LinkError, got %T", result)
	}
	if resultErr.Op != "origOp" || resultErr.Old != "/orig/old" || resultErr.New != "/orig/new" {
		t.Errorf("empty fields should preserve original, got %+v", *resultErr)
	}
}
