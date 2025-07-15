package testhelper

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestExecuteLines(t *testing.T) {
	tempDir := t.TempDir()

	lines := []string{
		"dir1/",
		"dir2/subdir/ 0o755",
		"file1.txt: \"hello world\"",
		"file2.txt: 0o600 \"restricted content\"",
	}

	err := ExecuteLines(tempDir, lines...)
	if err != nil {
		t.Fatalf("ExecuteLines failed: %v", err)
	}

	// Verify directories created
	info, err := fs.Stat(os.DirFS(tempDir), "dir1")
	if err != nil || !info.IsDir() {
		t.Errorf("dir1 not created properly")
	}

	info, err = fs.Stat(os.DirFS(tempDir), filepath.Join("dir2", "subdir"))
	if err != nil || !info.IsDir() {
		t.Errorf("dir2/subdir not created properly")
	}

	// Verify files created
	content, err := fs.ReadFile(os.DirFS(tempDir), "file1.txt")
	if err != nil {
		t.Errorf("failed to read file1.txt: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("file1.txt content mismatch: expected 'hello world', got %q", content)
	}

	content, err = fs.ReadFile(os.DirFS(tempDir), "file2.txt")
	if err != nil {
		t.Errorf("failed to read file2.txt: %v", err)
	}
	if string(content) != "restricted content" {
		t.Errorf("file2.txt content mismatch: expected 'restricted content', got %q", content)
	}
}

func TestExecuteLineOs_InvalidLine(t *testing.T) {
	tempDir := t.TempDir()

	err := ExecuteLineOs(tempDir, "invalid line without pattern")
	if err == nil {
		t.Errorf("expected error for invalid line, got nil")
	}
	if err != nil && err.Error() != `unknown line "invalid line without pattern"` {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestParseLine_Comprehensive(t *testing.T) {
	type testCase struct {
		name     string
		line     string
		expected LineDirection
	}

	cases := []testCase{
		// Directory tests
		{
			name: "simple directory",
			line: "dir/",
			expected: LineDirection{
				LineKind:   LineKindMkdir,
				Path:       "dir",
				Permission: 0,
			},
		},
		{
			name: "nested directory",
			line: "path/to/dir/",
			expected: LineDirection{
				LineKind:   LineKindMkdir,
				Path:       "path/to/dir",
				Permission: 0,
			},
		},
		// Basic file tests
		{
			name: "simple file",
			line: "file.txt: content",
			expected: LineDirection{
				LineKind:   LineKindWriteFile,
				Path:       "file.txt",
				Content:    []byte("content"),
				Permission: 0,
			},
		},
		// Symlink tests
		{
			name: "simple symlink",
			line: "link -> target",
			expected: LineDirection{
				LineKind:   LineKindSymlink,
				Path:       "link",
				TargetPath: "target",
			},
		},
		{
			name: "symlink with paths",
			line: "path/to/link -> ../target",
			expected: LineDirection{
				LineKind:   LineKindSymlink,
				Path:       "path/to/link",
				TargetPath: "../target",
			},
		},
		{
			name: "symlink with arrow in target",
			line: "link -> target -> nested",
			expected: LineDirection{
				LineKind:   LineKindSymlink,
				Path:       "link",
				TargetPath: "target -> nested",
			},
		},
		// Edge cases
		{
			name:     "unknown pattern",
			line:     "no pattern here",
			expected: LineDirection{},
		},
		{
			name: "file with text that doesn't look like permission",
			line: "bad.txt: notanumber",
			expected: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0,
				Path:       "bad.txt",
				Content:    []byte("notanumber"), // Single word content without spaces
			},
		},
		{
			name: "file with unquoted spaces should fail",
			line: "bad.txt: content with spaces",
			expected: LineDirection{}, // Should fail because spaces in content require quotes
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := ParseLine(tc.line)
			if !tc.expected.Equal(result) {
				t.Errorf("ParseLine mismatch:\nexpected: %#v\nactual:   %#v", tc.expected, result)
			}
		})
	}
}

func TestLineDirection_Equal(t *testing.T) {
	base := LineDirection{
		LineKind:   LineKindWriteFile,
		Permission: 0o644,
		Path:       "test.txt",
		TargetPath: "",
		Content:    []byte("content"),
	}

	type testCase struct {
		name     string
		other    LineDirection
		expected bool
	}

	cases := []testCase{
		{
			name:     "identical",
			other:    base,
			expected: true,
		},
		{
			name: "different LineKind",
			other: LineDirection{
				LineKind:   LineKindMkdir,
				Permission: 0o644,
				Path:       "test.txt",
				Content:    []byte("content"),
			},
			expected: false,
		},
		{
			name: "different Permission",
			other: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0o755,
				Path:       "test.txt",
				Content:    []byte("content"),
			},
			expected: false,
		},
		{
			name: "different Path",
			other: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0o644,
				Path:       "other.txt",
				Content:    []byte("content"),
			},
			expected: false,
		},
		{
			name: "different Content",
			other: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0o644,
				Path:       "test.txt",
				Content:    []byte("different"),
			},
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := base.Equal(tc.other)
			if result != tc.expected {
				t.Errorf("Equal() = %v, expected %v", result, tc.expected)
			}
		})
	}
	
	// Test symlink path cleaning
	t.Run("symlink path cleaning", func(t *testing.T) {
		link1 := LineDirection{
			LineKind:   LineKindSymlink,
			Path:       "link",
			TargetPath: "target",
		}
		link2 := LineDirection{
			LineKind:   LineKindSymlink,
			Path:       "link",
			TargetPath: "./target/",
		}
		if !link1.Equal(link2) {
			t.Errorf("symlinks with equivalent cleaned targets should be equal")
		}
	})
}

func TestLineDirection_MustExecuteOs(t *testing.T) {
	// Test that panic occurs
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("MustExecuteOs should panic on error")
		}
	}()

	// This should panic because we're trying to create a file in a non-existent directory
	l := LineDirection{
		LineKind: LineKindWriteFile,
		Path:     "/non/existent/path/file.txt",
		Content:  []byte("test"),
	}
	l.MustExecuteOs("/")
}

func TestExecuteLineDirection(t *testing.T) {
	type testCase struct {
		name      string
		direction LineDirection
		setupMock func(*mockPrepareFsys)
		wantError bool
	}

	cases := []testCase{
		{
			name: "create directory",
			direction: LineDirection{
				LineKind:   LineKindMkdir,
				Path:       "test/dir",
				Permission: 0o755,
			},
			setupMock: func(m *mockPrepareFsys) {
				m.mkdirAllFunc = func(path string, perm fs.FileMode) error {
					return nil
				}
				m.chmodFunc = func(path string, mode fs.FileMode) error {
					return nil
				}
			},
			wantError: false,
		},
		{
			name: "create directory with error",
			direction: LineDirection{
				LineKind: LineKindMkdir,
				Path:     "test/dir",
			},
			setupMock: func(m *mockPrepareFsys) {
				m.mkdirAllFunc = func(path string, perm fs.FileMode) error {
					return errors.New("mkdir failed")
				}
			},
			wantError: true,
		},
		{
			name: "write file",
			direction: LineDirection{
				LineKind:   LineKindWriteFile,
				Path:       "test.txt",
				Content:    []byte("hello"),
				Permission: 0o644,
			},
			setupMock: func(m *mockPrepareFsys) {
				m.createFunc = func(path string) (*mockPrepareFile, error) {
					return &mockPrepareFile{}, nil
				}
				m.chmodFunc = func(path string, mode fs.FileMode) error {
					return nil
				}
			},
			wantError: false,
		},
		{
			name: "write file with create error",
			direction: LineDirection{
				LineKind: LineKindWriteFile,
				Path:     "test.txt",
				Content:  []byte("hello"),
			},
			setupMock: func(m *mockPrepareFsys) {
				m.createFunc = func(path string) (*mockPrepareFile, error) {
					return nil, errors.New("create failed")
				}
			},
			wantError: true,
		},
		{
			name: "write file with write error",
			direction: LineDirection{
				LineKind: LineKindWriteFile,
				Path:     "test.txt",
				Content:  []byte("hello"),
			},
			setupMock: func(m *mockPrepareFsys) {
				m.createFunc = func(path string) (*mockPrepareFile, error) {
					return &mockPrepareFile{
						writeErr: errors.New("write failed"),
					}, nil
				}
			},
			wantError: true,
		},
		{
			name: "create symlink",
			direction: LineDirection{
				LineKind:   LineKindSymlink,
				Path:       "link",
				TargetPath: "target",
			},
			setupMock: func(m *mockPrepareFsys) {
				m.symlinkFunc = func(oldname, newname string) error {
					return nil
				}
			},
			wantError: false,
		},
		{
			name: "unknown line kind",
			direction: LineDirection{
				LineKind: "unknown",
			},
			setupMock: func(m *mockPrepareFsys) {},
			wantError: false, // Returns nil for unknown
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fsys := &mockPrepareFsys{}
			tc.setupMock(fsys)

			err := ExecuteLineDirection(fsys, tc.direction)
			if (err != nil) != tc.wantError {
				t.Errorf("ExecuteLineDirection() error = %v, wantError %v", err, tc.wantError)
			}

			// Verify calls for successful cases
			if !tc.wantError && err == nil {
				switch tc.direction.LineKind {
				case LineKindMkdir:
					if len(fsys.mkdirAllCalls) != 1 {
						t.Errorf("expected 1 MkdirAll call, got %d: %v", len(fsys.mkdirAllCalls), fsys.mkdirAllCalls)
					}
					if len(fsys.chmodCalls) != 1 {
						t.Errorf("expected 1 Chmod call, got %d: %v", len(fsys.chmodCalls), fsys.chmodCalls)
					}
				case LineKindWriteFile:
					if len(fsys.createCalls) != 1 {
						t.Errorf("expected 1 Create call, got %d: %v", len(fsys.createCalls), fsys.createCalls)
					}
					if len(fsys.chmodCalls) != 1 {
						t.Errorf("expected 1 Chmod call, got %d: %v", len(fsys.chmodCalls), fsys.chmodCalls)
					}
				case LineKindSymlink:
					if len(fsys.symlinkCalls) != 1 {
						t.Errorf("expected 1 Symlink call, got %d: %v", len(fsys.symlinkCalls), fsys.symlinkCalls)
					}
				}
			}
		})
	}
}

func TestFilterLineDirection(t *testing.T) {
	directions := []LineDirection{
		{LineKind: LineKindMkdir, Path: "dir1"},
		{LineKind: LineKindWriteFile, Path: "file1.txt"},
		{LineKind: LineKindMkdir, Path: "dir2"},
		{LineKind: LineKindSymlink, Path: "link1"},
		{LineKind: LineKindWriteFile, Path: "file2.txt"},
	}

	seq := slices.Values(directions)

	// Filter only directories
	filtered := FilterLineDirection(func(l LineDirection) bool {
		return l.LineKind == LineKindMkdir
	}, seq)

	var result []LineDirection
	for d := range filtered {
		result = append(result, d)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 directories, got %d", len(result))
	}
	for _, d := range result {
		if d.LineKind != LineKindMkdir {
			t.Errorf("expected only LineKindMkdir, got %s", d.LineKind)
		}
	}
}

func TestExecuteAllLineDirection(t *testing.T) {
	directions := []LineDirection{
		{LineKind: LineKindMkdir, Path: "dir1", Permission: 0o755},
		{LineKind: LineKindWriteFile, Path: "file1.txt", Content: []byte("test")},
		{LineKind: LineKindSymlink, Path: "link1", TargetPath: "file1.txt"},
	}

	t.Run("all successful", func(t *testing.T) {
		fsys := &mockPrepareFsys{
			mkdirAllFunc: func(path string, perm fs.FileMode) error { return nil },
			createFunc:   func(path string) (*mockPrepareFile, error) { return &mockPrepareFile{}, nil },
			chmodFunc:    func(path string, mode fs.FileMode) error { return nil },
			symlinkFunc:  func(oldname, newname string) error { return nil },
		}

		err := ExecuteAllLineDirection(fsys, slices.Values(directions))
		if err != nil {
			t.Errorf("ExecuteAllLineDirection() unexpected error: %v", err)
		}
	})

	t.Run("error stops execution", func(t *testing.T) {
		executedCount := 0
		fsys := &mockPrepareFsys{
			mkdirAllFunc: func(path string, perm fs.FileMode) error {
				executedCount++
				return nil
			},
			chmodFunc: func(path string, mode fs.FileMode) error { return nil },
			createFunc: func(path string) (*mockPrepareFile, error) {
				return nil, errors.New("create failed") // Second direction fails
			},
		}

		err := ExecuteAllLineDirection(fsys, slices.Values(directions))
		if err == nil {
			t.Errorf("ExecuteAllLineDirection() expected error, got nil")
		}
		// Only first mkdir should have been executed successfully
		if executedCount != 1 {
			t.Errorf("expected first direction to execute, but got %d mkdir calls", executedCount)
		}
	})
}

func TestLineDirection_ExecuteOs_CrossPlatform(t *testing.T) {
	if runtime.GOOS == "plan9" {
		t.Skip("symlinks not supported on plan9")
	}

	tempDir := t.TempDir()

	// Test symlink creation
	target := "target.txt"
	err := os.WriteFile(filepath.Join(tempDir, target), []byte("target content"), 0o644)
	if err != nil {
		t.Fatalf("failed to create target file: %v", err)
	}

	l := LineDirection{
		LineKind:   LineKindSymlink,
		Path:       "link",
		TargetPath: target,
	}

	err = l.ExecuteOs(tempDir)
	if err != nil {
		t.Fatalf("ExecuteOs failed for symlink: %v", err)
	}

	// Verify symlink
	linkPath := filepath.Join(tempDir, "link")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("failed to stat symlink: %v", err)
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("created file is not a symlink")
	}
}

// Mock implementations for testing

type mockPrepareFsys struct {
	chmodFunc    func(path string, mode fs.FileMode) error
	createFunc   func(path string) (*mockPrepareFile, error)
	mkdirAllFunc func(path string, perm fs.FileMode) error
	symlinkFunc  func(oldname, newname string) error

	// Track calls
	chmodCalls    []chmodCall
	createCalls   []string
	mkdirAllCalls []mkdirAllCall
	symlinkCalls  []symlinkCall
}

type chmodCall struct {
	path string
	mode fs.FileMode
}

type mkdirAllCall struct {
	path string
	perm fs.FileMode
}

type symlinkCall struct {
	oldname string
	newname string
}

func (m *mockPrepareFsys) Chmod(path string, mode fs.FileMode) error {
	m.chmodCalls = append(m.chmodCalls, chmodCall{path, mode})
	if m.chmodFunc != nil {
		return m.chmodFunc(path, mode)
	}
	return nil
}

func (m *mockPrepareFsys) Create(path string) (*mockPrepareFile, error) {
	m.createCalls = append(m.createCalls, path)
	if m.createFunc != nil {
		return m.createFunc(path)
	}
	return &mockPrepareFile{}, nil
}

func (m *mockPrepareFsys) MkdirAll(path string, perm fs.FileMode) error {
	m.mkdirAllCalls = append(m.mkdirAllCalls, mkdirAllCall{path, perm})
	if m.mkdirAllFunc != nil {
		return m.mkdirAllFunc(path, perm)
	}
	return nil
}

func (m *mockPrepareFsys) Symlink(oldname, newname string) error {
	m.symlinkCalls = append(m.symlinkCalls, symlinkCall{oldname, newname})
	if m.symlinkFunc != nil {
		return m.symlinkFunc(oldname, newname)
	}
	return nil
}

type mockPrepareFile struct {
	written  []byte
	closed   bool
	writeErr error
	closeErr error
}

func (m *mockPrepareFile) Write(p []byte) (int, error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	m.written = append(m.written, p...)
	return len(p), nil
}

func (m *mockPrepareFile) Close() error {
	if m.closed {
		return errors.New("already closed")
	}
	m.closed = true
	return m.closeErr
}