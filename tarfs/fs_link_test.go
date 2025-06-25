//go:build go1.25

package tarfs

import (
	"bytes"
	_ "embed"
	"errors"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/ngicks/go-fsys-helper/fsutil"
)

var (
	//go:embed testdata/test_symlink.tar.gz
	symlinkGzBin []byte
	//go:embed testdata/test_hardlink.tar.gz
	hardlinkGzBin []byte
)

/*
structure is like that
.
├── outside
│   ├── dir
│   │   └── nested_outside.txt
│   └── outside_file.txt
└── root

	└── readable
	    ├── file1.txt
	    ├── file2.txt
	    ├── subdir
	    │   ├── double_nested
	    │   │   └── double_nested.txt
	    │   ├── nested_file.txt
	    │   ├── symlink_upward -> ../symlink_inner
	    │   └── symlink_upward_escapes -> ../symlink_escapes
	    ├── symlink_escapes -> ../../outside/outside_file.txt
	    ├── symlink_escapes_dir -> ../../outside/dir
	    ├── symlink_inner -> ./file1.txt
	    └── symlink_inner_dir -> ./subdir
*/
var symlinkBin = ungzip(symlinkGzBin)

var (
	symlinkBinSeenExpected = []string{
		"outside/dir/nested_outside.txt",
		"outside/outside_file.txt",
		"root/readable/file1.txt",
		"root/readable/file2.txt",
		"root/readable/subdir/nested_file.txt",
		"root/readable/subdir/double_nested/double_nested.txt",
	}
	symlinkBinSeenExpectedSymlinked = []string{
		"root/readable/symlink_escapes",
		"root/readable/symlink_escapes_dir",
		"root/readable/symlink_inner",
		"root/readable/symlink_inner_dir",
		"root/readable/subdir/symlink_upward",
		"root/readable/subdir/symlink_upward_escapes",
	}
)

func TestFs_symlink(t *testing.T) {
	fsys, err := New(bytes.NewReader(symlinkBin), nil)
	if err != nil {
		panic(err)
	}
	if err := fstest.TestFS(fsys, symlinkBinSeenExpected...); err != nil {
		t.Errorf("fstest.TestFS fail: %v", err)
	}

	fsys, err = New(bytes.NewReader(symlinkBin), &FsOption{HandleSymlink: true})
	if err != nil {
		panic(err)
	}
	if err := fstest.TestFS(fsys, slices.Concat(symlinkBinSeenExpected, symlinkBinSeenExpectedSymlinked)...); err != nil {
		t.Errorf("fstest.TestFS fail: %v", err)
	}

	t.Run("resolution", func(t *testing.T) {
		type testCase struct {
			path, content string
		}
		for _, tc := range []testCase{
			{"root/readable/subdir/symlink_upward", "bazbazbaz"},
			{"root/readable/symlink_escapes_dir/nested_outside.txt", "barbarbar"},
		} {
			bin, err := fs.ReadFile(fsys, tc.path)
			if err != nil {
				panic(err)
			}

			expected := []byte(tc.content)
			if !bytes.Equal(expected, bin) {
				t.Errorf("not equal:expected(%q) != actual(%q)", string(expected), string(bin))
			}
		}
	})

	t.Run("dir", func(t *testing.T) {
		dirents, err := fs.ReadDir(fsys, "root/readable/symlink_escapes_dir")
		if err != nil {
			t.Fatalf("open failed for %q: %v", "root/readable/symlink_escapes_dir", err)
		}
		names := make([]string, len(dirents))
		for i, dirent := range dirents {
			names[i] = dirent.Name()
		}
		expected := []string{"nested_outside.txt"}
		if !slices.Equal(expected, names) {
			t.Errorf("not equal:expected(%#v) != actual(%#v)", expected, names)
		}
	})
}

func TestFs_symlink_path_ecapes(t *testing.T) {
	type testCase struct {
		name    func() string
		rooted  bool
		err     error
		content []byte
	}

	for _, tc := range []testCase{
		{
			func() string { return "rooted" },
			true,
			fsutil.ErrPathEscapes,
			nil,
		},
		{
			func() string { return "unrooted" },
			false,
			nil,
			[]byte("foofoofoo"),
		},
	} {
		t.Run(tc.name(), func(t *testing.T) {
			fsys, err := New(bytes.NewReader(symlinkBin), &FsOption{HandleSymlink: true, IsRooted: tc.rooted})
			if err != nil {
				panic(err)
			}
			sub, err := fsys.Sub("root/readable")
			if err != nil {
				panic(err)
			}
			bin, err := fs.ReadFile(sub, "symlink_escapes")
			if !errors.Is(err, tc.err) {
				t.Fatalf("should be %q but is %a", tc.err, err)
			}
			if err == nil {
				if !bytes.Equal(tc.content, bin) {
					t.Errorf("not equal: expected(%q) != actual(%q)", string(tc.content), string(bin))
				}
			}
		})
	}

	t.Run("unrooted", func(t *testing.T) {
		fsys, err := New(bytes.NewReader(symlinkBin), &FsOption{HandleSymlink: true, IsRooted: false})
		if err != nil {
			panic(err)
		}
		sub, err := fsys.Sub("root/readable")
		if err != nil {
			panic(err)
		}
		bin, err := fs.ReadFile(sub, "symlink_escapes")
		if err != nil {
			t.Fatalf("failed with %v", err)
		}

		expected := []byte("foofoofoo")
		if !bytes.Equal(expected, bin) {
			t.Errorf("not equal: expected(%q) != actual(%q)", string(expected), string(bin))
		}
	})
}

/*
foo.txt and link is linked to same file(hard link).
.
├── foo.txt
└── sub

	├── link
	└── sub
	    └── link
*/
var (
	hardlinkBin             = ungzip(hardlinkGzBin)
	hardlinkBinSeenExpected = []string{
		"foo.txt",
		"sub/link",
		"sub/sub/link",
	}
)

func TestFs_hardlink(t *testing.T) {
	fsys, err := New(bytes.NewReader(hardlinkBin), nil)
	if err != nil {
		panic(err)
	}
	if err := fstest.TestFS(fsys, hardlinkBinSeenExpected...); err != nil {
		t.Errorf("fstest.TestFS fail: %v", err)
	}
}
