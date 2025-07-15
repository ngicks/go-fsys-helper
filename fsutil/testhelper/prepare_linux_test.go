package testhelper

import (
	"bytes"
	"io/fs"
	"os"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/internal/osfslite"
)

func TestParseLine(t *testing.T) {
	type testCase struct {
		line     string
		expected LineDirection
	}

	cases := []testCase{
		// Basic file cases
		{
			line: "foo1: yayyay",
			expected: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0,
				Path:       "foo1",
				Content:    []byte("yayyay"),
			},
		},
		{
			line: "foo2: 0o77755 yayyay",
			expected: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0o77755,
				Path:       "foo2",
				Content:    []byte("yayyay"),
			},
		},
		// Directory cases
		{
			line: "bar1/ yayyay",
			expected: LineDirection{
				LineKind:   LineKindMkdir,
				Permission: 0,
				Path:       "bar1",
			},
		},
		{
			line: "bar2/bar2/bar2/ 457",
			expected: LineDirection{
				LineKind:   LineKindMkdir,
				Permission: 0o711,
				Path:       "bar2/bar2/bar2",
			},
		},
		// Symlink case
		{
			line: "baz -> foo1",
			expected: LineDirection{
				LineKind:   LineKindSymlink,
				Path:       "baz",
				TargetPath: "foo1",
			},
		},
		// Quoted content cases
		{
			line: `quoted.txt: "hello world with spaces"`,
			expected: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0,
				Path:       "quoted.txt",
				Content:    []byte("hello world with spaces"),
			},
		},
		{
			line: `secure.txt: 0o600 "secret content"`,
			expected: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0o600,
				Path:       "secure.txt",
				Content:    []byte("secret content"),
			},
		},
		{
			line: "raw.txt: `raw string content`",
			expected: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0,
				Path:       "raw.txt",
				Content:    []byte("raw string content"),
			},
		},
		{
			line: `escaped.txt: "content with \"quotes\" inside"`,
			expected: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0,
				Path:       "escaped.txt",
				Content:    []byte(`content with "quotes" inside`),
			},
		},
		// Permission edge cases
		{
			line: "bad.txt: 0o999 content", // 999 is not valid octal
			expected: LineDirection{}, // Should return empty because permission is malformed
		},
		{
			line: "numeric.txt: 384 content", // 384 decimal = 0o600 octal
			expected: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0o600,
				Path:       "numeric.txt",
				Content:    []byte("content"),
			},
		},
		{
			line: "hex.txt: 0x1a4 content", // 0x1a4 hex = 420 decimal = 0o644 octal
			expected: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0o644,
				Path:       "hex.txt",
				Content:    []byte("content"),
			},
		},
		// Content with colons and spaces - must be quoted
		{
			line: `config.yml: "key: value"`,
			expected: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0,
				Path:       "config.yml",
				Content:    []byte("key: value"),
			},
		},
		// Unquoted content with spaces should fail
		{
			line: "config.yml: key: value",
			expected: LineDirection{}, // Should fail because spaces in content require quotes
		},
		// Empty content
		{
			line: "empty.txt: ",
			expected: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0,
				Path:       "empty.txt",
				Content:    []byte{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			parsed := ParseLine(tc.line)
			if !tc.expected.Equal(parsed) {
				t.Errorf("not qeual:\nexpected: %#v\nactual  : %#v\n", tc.expected, parsed)
			}
		})
	}
}

func TestLineDirection_Execute(t *testing.T) {
	// can't execute this on windwos platform since it only paritally uses permission.

	tempDir := t.TempDir()

	fsys := osfslite.New(tempDir)

	type testCase struct {
		dir             LineDirection
		expectedMode    fs.FileMode
		expectedContent []byte
		expectedTarget  string
	}

	cases := []testCase{
		{
			dir: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0,
				Path:       "foo1",
				Content:    []byte("yayyay"),
			},
			expectedMode:    fs.ModePerm,
			expectedContent: []byte("yayyay"),
		},
		{
			dir: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0o77755, // bit range ot of 0o777 is clipped but within it, respected.
				Path:       "foo2",
				Content:    []byte("yayyay"),
			},
			expectedMode:    0o755,
			expectedContent: []byte("yayyay"),
		},
		{
			dir: LineDirection{
				LineKind:   LineKindMkdir,
				Permission: 0,
				Path:       "bar1",
				Content:    []byte("yayyay"), // content will be ignored
			},
			expectedMode: fs.ModeDir | fs.ModePerm,
		},
		{
			dir: LineDirection{
				LineKind:   LineKindMkdir,
				Permission: 0o711,
				Path:       "bar2/bar2/bar2",
			},
			expectedMode: fs.ModeDir | 0o711,
		},
		{
			dir: LineDirection{
				LineKind:   LineKindSymlink,
				Path:       "baz",
				TargetPath: "foo1",
			},
			expectedMode: fs.ModeSymlink | 0o777,
		},
		// Test quoted content with permissions
		{
			dir: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0o640,
				Path:       "quoted",
				Content:    []byte("content with spaces"),
			},
			expectedMode:    0o640,
			expectedContent: []byte("content with spaces"),
		},
		{
			dir: LineDirection{
				LineKind:   LineKindWriteFile,
				Permission: 0o755,
				Path:       "script.sh",
				Content:    []byte("#!/bin/bash\necho \"Hello World\""),
			},
			expectedMode:    0o755,
			expectedContent: []byte("#!/bin/bash\necho \"Hello World\""),
		},
	}
	for _, tc := range cases {
		t.Run(tc.dir.Path, func(t *testing.T) {
			err := ExecuteLineDirection(fsys, tc.dir)
			if err != nil {
				t.Fatalf("execute: %v", err)
			}

			s, err := fsys.Lstat(tc.dir.Path)
			if err != nil {
				t.Fatalf("stat: %v", err)
			}
			if tc.expectedMode != s.Mode() {
				t.Errorf("mode not equal: expected(%b) != actual(%b)", tc.expectedMode, s.Mode())
			}

			if tc.expectedContent != nil {
				read, err := fs.ReadFile(os.DirFS(tempDir), tc.dir.Path)
				if err != nil {
					t.Fatalf("read file: %v", err)
				}
				if !bytes.Equal(tc.expectedContent, read) {
					t.Errorf("file content not equal: expected(%s) != actual(%s)", string(tc.expectedContent), string(read))
				}
			}
			if tc.expectedTarget != "" {
				target, err := fsys.ReadLink(tc.dir.TargetPath)
				if err != nil {
					t.Fatalf("readlink: %v", err)
				}

				if tc.expectedTarget != target {
					t.Errorf(
						"link target not equal: expected(%s) != actual(%s)",
						tc.expectedTarget, target,
					)
				}
			}
		})
	}
}
