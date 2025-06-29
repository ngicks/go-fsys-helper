package paths

import (
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestPathFromHead(t *testing.T) {
	type testCase struct {
		name     string
		input    string
		expected []string
	}
	tests := []testCase{
		{
			name:     "single component",
			input:    "file.txt",
			expected: []string{"file.txt"},
		},
		{
			name:     "two components",
			input:    filepath.Join("dir", "file.txt"),
			expected: []string{"dir", filepath.Join("dir", "file.txt")},
		},
		{
			name:  "multiple components",
			input: filepath.Join("a", "b", "c", "file.txt"),
			expected: []string{
				"a",
				filepath.Join("a", "b"),
				filepath.Join("a", "b", "c"),
				filepath.Join("a", "b", "c", "file.txt"),
			},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{"."},
		},
		{
			name:     "current directory",
			input:    ".",
			expected: []string{"."},
		},
		{
			name:     "parent directory",
			input:    "..",
			expected: []string{".."},
		},
	}

	if runtime.GOOS == "windows" {
		tests = append(
			tests,
			testCase{
				name:  "with leading slash",
				input: "C:\\root\\dir\\file",
				expected: []string{
					"C:\\",
					"C:\\root",
					"C:\\root\\dir",
					"C:\\root\\dir\\file",
				},
			},
		)
	} else {
		tests = append(
			tests,
			testCase{
				name:  "with leading slash",
				input: "/root/dir/file",
				expected: []string{
					"/",
					"/root",
					"/root/dir",
					"/root/dir/file",
				},
			},
		)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := slices.Collect(PathFromHead(tt.input))
			if !slices.Equal(result, tt.expected) {
				t.Errorf("not equal:\nexpected: %v\nactual: %v", tt.expected, result)
			}
		})
	}
}

func TestPathFromTail(t *testing.T) {
	type testCase struct {
		name     string
		input    string
		expected []string
	}
	tests := []testCase{
		{
			name:     "single component",
			input:    "file.txt",
			expected: []string{"file.txt"},
		},
		{
			name:  "two components",
			input: filepath.Join("dir", "file.txt"),
			expected: []string{
				filepath.Join("dir", "file.txt"),
				"dir",
			},
		},
		{
			name:  "multiple components",
			input: filepath.Join("a", "b", "c", "file.txt"),
			expected: []string{
				filepath.Join("a", "b", "c", "file.txt"),
				filepath.Join("a", "b", "c"),
				filepath.Join("a", "b"),
				"a",
			},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{"."},
		},
		{
			name:     "current directory",
			input:    ".",
			expected: []string{"."},
		},
	}

	if runtime.GOOS == "windows" {
		tests = append(
			tests,
			testCase{
				name:  "with leading slash",
				input: "C:\\root\\dir\\file",
				expected: []string{
					"C:\\root\\dir\\file",
					"C:\\root\\dir",
					"C:\\root",
					"C:\\",
				},
			},
		)
	} else {
		tests = append(
			tests,
			testCase{
				name:  "with leading slash",
				input: "/root/dir/file",
				expected: []string{
					"/root/dir/file",
					"/root/dir",
					"/root",
					"/",
				},
			},
		)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result []string
			for path := range PathFromTail(tt.input) {
				result = append(result, path)
			}

			if !slices.Equal(result, tt.expected) {
				t.Errorf("not equal:\nexpected: %v\nactual: %v", tt.expected, result)
			}
		})
	}
}

func TestPathFromHead_StopEarly(t *testing.T) {
	// Test that iteration stops when yield returns false
	input := filepath.Join("a", "b", "c", "d", "e")
	var result []string

	for path := range PathFromHead(input) {
		result = append(result, path)
		if len(result) == 2 {
			break
		}
	}

	expected := []string{"a", filepath.Join("a", "b")}
	if !slices.Equal(result, expected) {
		t.Errorf("not equal:\nexpected: %v\nactual: %v", expected, result)
	}
}

func TestPathFromTail_StopEarly(t *testing.T) {
	// Test that iteration stops when yield returns false
	input := filepath.Join("a", "b", "c", "d", "e")
	var result []string

	for path := range PathFromTail(input) {
		result = append(result, path)
		if len(result) == 2 {
			break
		}
	}

	expected := []string{
		filepath.Join("a", "b", "c", "d", "e"),
		filepath.Join("a", "b", "c", "d"),
	}
	if !slices.Equal(result, expected) {
		t.Errorf("not equal:\nexpected: %v\nactual: %v", expected, result)
	}
}
