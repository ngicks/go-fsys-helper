//go:build unix || (js && wasm) || wasip1

package acceptancetest_test

import (
	"bytes"
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func TestParseLine(t *testing.T) {
	type testCase struct {
		line     string
		expected acceptancetest.LineDirection
	}

	cases := []testCase{
		{
			line: "foo1: yayyay",
			expected: acceptancetest.LineDirection{
				LineKind:   acceptancetest.LineKindWriteFile,
				Permission: 0,
				Path:       "foo1",
				Content:    []byte("yayyay"),
			},
		},
		{
			line: "foo2: 0o77755 yayyay",
			expected: acceptancetest.LineDirection{
				LineKind:   acceptancetest.LineKindWriteFile,
				Permission: 0o77755, // bit range ot of 0o777 is clipped but within it, respected.
				Path:       "foo2",
				Content:    []byte("yayyay"),
			},
		},
		{
			line: "bar1/ yayyay",
			expected: acceptancetest.LineDirection{
				LineKind:   acceptancetest.LineKindMkdir,
				Permission: 0,
				Path:       "bar1",
			},
		},
		{
			line: "bar2/bar2/bar2/ 457",
			expected: acceptancetest.LineDirection{
				LineKind:   acceptancetest.LineKindMkdir,
				Permission: 0o711, // bit range ot of 0o777 is clipped but within it, respected.
				Path:       "bar2/bar2/bar2",
			},
		},
		{
			line: "baz -> foo1",
			expected: acceptancetest.LineDirection{
				LineKind:   acceptancetest.LineKindSymlink,
				Path:       "baz",
				TargetPath: "foo1",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			parsed := acceptancetest.ParseLine(tc.line)
			if !tc.expected.Equal(parsed) {
				t.Errorf("not qeual:\nexpected: %#v\nactual  : %#v\n", tc.expected, parsed)
			}
		})
	}
}

func TestLineDirection_Execute(t *testing.T) {
	// can't execute this on windwos platform since it only paritally uses permission.

	tempDir := t.TempDir()

	fsys, err := osfs.NewRooted(tempDir)
	if err != nil {
		panic(err)
	}

	type testCase struct {
		dir             acceptancetest.LineDirection
		expectedMode    fs.FileMode
		expectedContent []byte
		expectedTarget  string
	}

	cases := []testCase{
		{
			dir: acceptancetest.LineDirection{
				LineKind:   acceptancetest.LineKindWriteFile,
				Permission: 0,
				Path:       "foo1",
				Content:    []byte("yayyay"),
			},
			expectedMode:    fs.ModePerm,
			expectedContent: []byte("yayyay"),
		},
		{
			dir: acceptancetest.LineDirection{
				LineKind:   acceptancetest.LineKindWriteFile,
				Permission: 0o77755, // bit range ot of 0o777 is clipped but within it, respected.
				Path:       "foo2",
				Content:    []byte("yayyay"),
			},
			expectedMode:    0o755,
			expectedContent: []byte("yayyay"),
		},
		{
			dir: acceptancetest.LineDirection{
				LineKind:   acceptancetest.LineKindMkdir,
				Permission: 0,
				Path:       "bar1",
				Content:    []byte("yayyay"), // content will be ignored
			},
			expectedMode: fs.ModeDir | fs.ModePerm,
		},
		{
			dir: acceptancetest.LineDirection{
				LineKind:   acceptancetest.LineKindMkdir,
				Permission: 0o711, // bit range ot of 0o777 is clipped but within it, respected.
				Path:       "bar2/bar2/bar2",
			},
			expectedMode: fs.ModeDir | 0o711,
		},
		{
			dir: acceptancetest.LineDirection{
				LineKind:   acceptancetest.LineKindSymlink,
				Path:       "baz",
				TargetPath: "foo1",
			},
			expectedMode: fs.ModeSymlink | 0o777,
		},
	}
	for _, tc := range cases {
		t.Run(tc.dir.Path, func(t *testing.T) {
			err := tc.dir.Execute(fsys)
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
				read, err := vroot.ReadFile(fsys, tc.dir.Path)
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
