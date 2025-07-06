//go:build unix || (js && wasm) || wasip1

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
				Permission: 0o77755, // bit range ot of 0o777 is clipped but within it, respected.
				Path:       "foo2",
				Content:    []byte("yayyay"),
			},
		},
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
				Permission: 0o711, // bit range ot of 0o777 is clipped but within it, respected.
				Path:       "bar2/bar2/bar2",
			},
		},
		{
			line: "baz -> foo1",
			expected: LineDirection{
				LineKind:   LineKindSymlink,
				Path:       "baz",
				TargetPath: "foo1",
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
				Permission: 0o711, // bit range ot of 0o777 is clipped but within it, respected.
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
