package fsutil

import (
	"io/fs"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMaskChmod(t *testing.T) {
	t.Run("MaskChmodModePlan9", func(t *testing.T) {
		type testCase struct {
			name     string
			input    fs.FileMode
			expected fs.FileMode
		}

		tests := []testCase{
			{
				name:     "regular file with permissions",
				input:    0o644,
				expected: 0o644,
			},
			{
				name:     "executable file",
				input:    0o755,
				expected: 0o755,
			},
			{
				name:     "file with append mode",
				input:    0o644 | os.ModeAppend,
				expected: 0o644 | os.ModeAppend,
			},
			{
				name:     "file with exclusive mode",
				input:    0o644 | os.ModeExclusive,
				expected: 0o644 | os.ModeExclusive,
			},
			{
				name:     "file with temporary mode",
				input:    0o644 | os.ModeTemporary,
				expected: 0o644 | os.ModeTemporary,
			},
			{
				name:     "file with setuid (should be masked out)",
				input:    0o644 | os.ModeSetuid,
				expected: 0o644,
			},
			{
				name:     "file with setgid (should be masked out)",
				input:    0o644 | os.ModeSetgid,
				expected: 0o644,
			},
			{
				name:     "file with sticky bit (should be masked out)",
				input:    0o644 | os.ModeSticky,
				expected: 0o644,
			},
			{
				name:     "directory with all plan9 supported modes",
				input:    os.ModeDir | os.ModePerm | os.ModeAppend | os.ModeExclusive | os.ModeTemporary,
				expected: os.ModePerm | os.ModeAppend | os.ModeExclusive | os.ModeTemporary,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				actual := MaskChmodModePlan9(tc.input)
				if actual != tc.expected {
					t.Errorf("not equal: expected(%o) != actual(%o)", tc.expected, actual)
				}
			})
		}
	})

	t.Run("MaskChmodModeUnix", func(t *testing.T) {
		type testCase struct {
			name     string
			input    fs.FileMode
			expected fs.FileMode
		}

		tests := []testCase{
			{
				name:     "regular file with permissions",
				input:    0o644,
				expected: 0o644,
			},
			{
				name:     "executable file",
				input:    0o755,
				expected: 0o755,
			},
			{
				name:     "file with setuid",
				input:    os.ModeSetuid | 0o755,
				expected: os.ModeSetuid | 0o755,
			},
			{
				name:     "file with setgid",
				input:    os.ModeSetgid | 0o755,
				expected: os.ModeSetgid | 0o755,
			},
			{
				name:     "directory with sticky bit",
				input:    os.ModeDir | os.ModeSticky | 0o777,
				expected: os.ModeSticky | 0o777,
			},
			{
				name:     "file with all unix special bits",
				input:    os.ModeSetuid | os.ModeSetgid | os.ModeSticky | 0o777,
				expected: os.ModeSetuid | os.ModeSetgid | os.ModeSticky | 0o777,
			},
			{
				name:     "file with append mode (should be masked out)",
				input:    0o644 | os.ModeAppend,
				expected: 0o644,
			},
			{
				name:     "file with exclusive mode (should be masked out)",
				input:    0o644 | os.ModeExclusive,
				expected: 0o644,
			},
			{
				name:     "file with temporary mode (should be masked out)",
				input:    0o644 | os.ModeTemporary,
				expected: 0o644,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				actual := MaskChmodModeUnix(tc.input)
				if actual != tc.expected {
					t.Errorf("not equal: expected(%o) != actual(%o)", tc.expected, actual)
				}
			})
		}
	})

	t.Run("MaskChmodModeWindows", func(t *testing.T) {
		type testCase struct {
			name     string
			input    fs.FileMode
			expected fs.FileMode
		}

		tests := []testCase{
			// File tests
			{
				name:     "writable file (0o200 bit set)",
				input:    0o644,
				expected: 0o666,
			},
			{
				name:     "writable file with various permissions",
				input:    0o755,
				expected: 0o666,
			},
			{
				name:     "read-only file (0o200 bit not set)",
				input:    0o444,
				expected: 0o444,
			},
			{
				name:     "read-only file with execute bits",
				input:    0o555,
				expected: 0o444,
			},
			// Directory tests
			{
				name:     "writable directory",
				input:    os.ModeDir | 0o755,
				expected: os.ModeDir | 0o777,
			},
			{
				name:     "writable directory with limited permissions",
				input:    os.ModeDir | 0o644,
				expected: os.ModeDir | 0o777,
			},
			{
				name:     "read-only directory",
				input:    os.ModeDir | 0o555,
				expected: os.ModeDir | 0o555,
			},
			{
				name:     "read-only directory with no permissions",
				input:    os.ModeDir | 0o000,
				expected: os.ModeDir | 0o555,
			},
			// Edge cases
			{
				name:     "file with only 0o200 bit",
				input:    0o200,
				expected: 0o666,
			},
			{
				name:     "file with only 0o400 bit",
				input:    0o400,
				expected: 0o444,
			},
			{
				name:     "file with special modes (should be preserved)",
				input:    0o644 | os.ModeSetuid | os.ModeSetgid,
				expected: os.ModeSetuid | os.ModeSetgid | 0o666,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				actual := MaskChmodModeWindows(tc.input)
				if actual != tc.expected {
					t.Errorf("not equal: expected(%o) != actual(%o)", tc.expected, actual)
				}
			})
		}
	})

	t.Run("MaskChmodMode platform-specific", func(t *testing.T) {
		// Test that MaskChmodMode calls the correct platform-specific function
		type testCase struct {
			name     string
			input    fs.FileMode
			expected string // Human-readable format
		}

		var tests []testCase

		switch runtime.GOOS {
		case "windows":
			tests = []testCase{
				{
					name:     "windows writable file",
					input:    0o644,
					expected: "-rw-rw-rw-",
				},
				{
					name:     "windows read-only file",
					input:    0o444,
					expected: "-r--r--r--",
				},
				{
					name:     "windows directory",
					input:    os.ModeDir | 0o755,
					expected: "drwxrwxrwx",
				},
				{
					name:     "windows read-only directory",
					input:    os.ModeDir | 0o444,
					expected: "dr-xr-xr-x",
				},
			}
		case "plan9":
			tests = []testCase{
				{
					name:     "plan9 file with append",
					input:    0o644 | os.ModeAppend,
					expected: "arw-r--r--",
				},
				{
					name:     "plan9 file with setuid (masked out)",
					input:    0o644 | os.ModeSetuid,
					expected: "-rw-r--r--",
				},
			}
		default: // unix
			tests = []testCase{
				{
					name:     "unix file with setuid",
					input:    os.ModeSetuid | 0o755,
					expected: "urwxr-xr-x",
				},
				{
					name:     "unix file with sticky bit",
					input:    os.ModeSticky | 0o777,
					expected: "trwxrwxrwx",
				},
				{
					name:     "unix file with append (masked out)",
					input:    0o644 | os.ModeAppend,
					expected: "-rw-r--r--",
				},
			}
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				actual := MaskChmodMode(tc.input)
				actualInfo := &mockFileInfo{mode: actual}
				actualFormatted := fs.FormatFileInfo(actualInfo)
				actualMode, _, _ := strings.Cut(actualFormatted, " ")

				if actualMode != tc.expected {
					t.Errorf("not equal: expected(%s) != actual(%s)", tc.expected, actualMode)
				}
			})
		}
	})

	t.Run("ChmodMask constant", func(t *testing.T) {
		// Test that ChmodMask constant has the expected value for each platform
		switch runtime.GOOS {
		case "windows":
			if ChmodMask != ChmodMaskWindows {
				t.Errorf("ChmodMask on Windows: expected %o, got %o", ChmodMaskWindows, ChmodMask)
			}
		case "plan9":
			if ChmodMask != ChmodMaskPlan9 {
				t.Errorf("ChmodMask on Plan9: expected %o, got %o", ChmodMaskPlan9, ChmodMask)
			}
		default: // unix
			if ChmodMask != ChmodMaskUnix {
				t.Errorf("ChmodMask on Unix: expected %o, got %o", ChmodMaskUnix, ChmodMask)
			}
		}
	})
}

// mockFileInfo is a minimal implementation of fs.FileInfo for testing
type mockFileInfo struct {
	mode fs.FileMode
}

func (m *mockFileInfo) Name() string       { return "test" }
func (m *mockFileInfo) Size() int64        { return 0 }
func (m *mockFileInfo) Mode() fs.FileMode  { return m.mode }
func (m *mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m *mockFileInfo) IsDir() bool        { return m.mode.IsDir() }
func (m *mockFileInfo) Sys() any           { return nil }

