package testhelper

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil/internal/osfslite"
)

func TestParseSetupProcLine(t *testing.T) {
	type testCase struct {
		name    string
		line    string
		wantErr bool
		assert  func(t *testing.T, proc SetupProc[*os.File, *osfslite.OsfsLite])
	}

	cases := []testCase{
		{
			name: "directory",
			line: "dir/sub/",
			assert: func(t *testing.T, proc SetupProc[*os.File, *osfslite.OsfsLite]) {
				got, ok := proc.(*CreateDir[*os.File, *osfslite.OsfsLite])
				if !ok {
					t.Fatalf("proc type = %T, want *CreateDir", proc)
				}
				if got.Path() != filepath.Join("dir", "sub") {
					t.Fatalf("path = %q", got.Path())
				}
			},
		},
		{
			name: "file",
			line: `file.txt: "hello world"`,
			assert: func(t *testing.T, proc SetupProc[*os.File, *osfslite.OsfsLite]) {
				got, ok := proc.(*CreateFile[*os.File, *osfslite.OsfsLite])
				if !ok {
					t.Fatalf("proc type = %T, want *CreateFile", proc)
				}
				if got.Path() != "file.txt" || !bytes.Equal(got.Content, []byte("hello world")) {
					t.Fatalf("file = (%q, %q)", got.Path(), got.Content)
				}
			},
		},
		{
			name: "symlink",
			line: "link -> target",
			assert: func(t *testing.T, proc SetupProc[*os.File, *osfslite.OsfsLite]) {
				got, ok := proc.(*CreateSymlink[*os.File, *osfslite.OsfsLite])
				if !ok {
					t.Fatalf("proc type = %T, want *CreateSymlink", proc)
				}
				if got.Path() != "link" || got.TargetPath != "target" {
					t.Fatalf("symlink = (%q, %q)", got.Path(), got.TargetPath)
				}
			},
		},
		{
			name:    "mode is rejected",
			line:    "file.txt: 0o600 content",
			wantErr: true,
		},
		{
			name:    "unknown",
			line:    "plain text",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proc, err := ParseSetupProcLine[*os.File, *osfslite.OsfsLite](tc.line)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseSetupProcLine() error = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil {
				tc.assert(t, proc)
			}
		})
	}
}

func TestCSetup(t *testing.T) {
	tempDir := t.TempDir()
	fsys := osfslite.New(tempDir)
	c := New(t, fsys)

	mtime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	c.Setup(
		&CreateLink[*os.File, *osfslite.OsfsLite]{
			Name:       filepath.Join("links", "hard"),
			TargetPath: filepath.Join("dir", "file.txt"),
		},
		&CreateFile[*os.File, *osfslite.OsfsLite]{
			Name:    filepath.Join("dir", "file.txt"),
			Mtime:   mtime,
			Content: []byte("content"),
		},
		&CreateDir[*os.File, *osfslite.OsfsLite]{
			Name: filepath.Join("dir", "nested"),
		},
	)

	read, err := os.ReadFile(filepath.Join(tempDir, "dir", "file.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(read, []byte("content")) {
		t.Fatalf("content = %q", read)
	}

	info, err := os.Stat(filepath.Join(tempDir, "dir", "file.txt"))
	if err != nil {
		t.Fatalf("Stat file: %v", err)
	}
	if !info.ModTime().Equal(mtime) {
		t.Fatalf("mtime = %s, want %s", info.ModTime(), mtime)
	}

	linkInfo, err := os.Stat(filepath.Join(tempDir, "links", "hard"))
	if err != nil {
		t.Fatalf("Stat hardlink: %v", err)
	}
	if !os.SameFile(info, linkInfo) {
		t.Fatalf("hardlink does not point to source file")
	}

	dirInfo, err := fs.Stat(os.DirFS(tempDir), "dir/nested")
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	if !dirInfo.IsDir() {
		t.Fatalf("dir/nested is not a directory")
	}
}
