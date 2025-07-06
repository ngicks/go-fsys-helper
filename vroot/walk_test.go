package vroot_test

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

type pathSeen struct {
	path     string
	realPath string
}

func (s pathSeen) ToSlash() pathSeen {
	return pathSeen{filepath.ToSlash(s.path), filepath.ToSlash(s.realPath)}
}

func (s pathSeen) Compare(s2 pathSeen) int {
	return cmp.Compare(s.path, s2.path)
}

func mapIter[V1, V2 any](fn func(v1 V1) V2, seq iter.Seq[V1]) iter.Seq[V2] {
	return func(yield func(V2) bool) {
		for v := range seq {
			if !yield(fn(v)) {
				return
			}
		}
	}
}

func collectString(prefix, suffix string, seq iter.Seq[string]) string {
	var builder strings.Builder
	for s := range seq {
		builder.WriteString(prefix)
		builder.WriteString(s)
		builder.WriteString(suffix)
	}
	return builder.String()
}

func assertPathSeen(t *testing.T, expected, actual []pathSeen) {
	t.Helper()
	convertedExpected := slices.SortedFunc(
		mapIter(pathSeen.ToSlash, slices.Values(expected)),
		pathSeen.Compare,
	)
	convertedActual := slices.SortedFunc(
		mapIter(pathSeen.ToSlash, slices.Values(actual)),
		pathSeen.Compare,
	)
	if !slices.Equal(
		convertedExpected,
		convertedActual,
	) {
		onlyInExpected := slices.DeleteFunc(slices.Clone(convertedExpected), func(p pathSeen) bool { return slices.Contains(convertedActual, p) })
		onlyInActual := slices.DeleteFunc(slices.Clone(convertedActual), func(p pathSeen) bool { return slices.Contains(convertedExpected, p) })
		t.Fatalf(
			"not equal:\n"+
				"expected: %#v\n"+
				"actual  :%#v\n"+
				"diff:(- exists only in expected, + exists only in actual)\n"+
				"%s\n"+
				"%s\n",
			convertedExpected,
			convertedActual,
			collectString(
				"- ", "\n",
				mapIter(
					func(p pathSeen) string { return fmt.Sprintf("%#v", p) },
					slices.Values(onlyInExpected),
				),
			),
			collectString(
				"+ ", "\n",
				mapIter(
					func(p pathSeen) string { return fmt.Sprintf("%#v", p) },
					slices.Values(onlyInActual),
				),
			),
		)
	}
}

func TestWalk_Unrooted_no_loop(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	acceptancetest.MakeOsFsys(tempDir, true, false)
	r, err := osfs.NewUnrooted(filepath.Join(tempDir, "root", "readable"))
	if err != nil {
		panic(err)
	}
	defer r.Close()

	t.Run("symlink not follow", func(t *testing.T) {
		var seen []pathSeen
		err := vroot.WalkDir(
			r,
			".",
			nil,
			func(path, realPath string, d fs.FileInfo, err error) error {
				if err != nil {
					return err
				}
				seen = append(seen, pathSeen{path, realPath})
				return nil
			},
		)
		if err != nil {
			t.Fatalf("walk failed: %v", err)
		}
		expected := []pathSeen{
			{path: ".", realPath: "."},
			{path: "symlink_escapes_dir", realPath: "symlink_escapes_dir"},
			{path: "file1.txt", realPath: "file1.txt"},
			{path: "symlink_inner_dir", realPath: "symlink_inner_dir"},
			{path: "symlink_inner", realPath: "symlink_inner"},
			{path: "subdir", realPath: "subdir"},
			{path: "subdir/symlink_upward_escapes", realPath: "subdir/symlink_upward_escapes"},
			{path: "subdir/nested_file.txt", realPath: "subdir/nested_file.txt"},
			{path: "subdir/double_nested", realPath: "subdir/double_nested"},
			{path: "subdir/double_nested/double_nested.txt", realPath: "subdir/double_nested/double_nested.txt"},
			{path: "subdir/symlink_upward", realPath: "subdir/symlink_upward"},
			{path: "file2.txt", realPath: "file2.txt"},
			{path: "symlink_escapes", realPath: "symlink_escapes"},
		}
		assertPathSeen(t, expected, seen)
	})
	t.Run("symlink follow", func(t *testing.T) {
		var seen []pathSeen
		err := vroot.WalkDir(
			r,
			".",
			&vroot.WalkOption{ResolveSymlink: true},
			func(path, realPath string, d fs.FileInfo, err error) error {
				if err != nil {
					return err
				}
				seen = append(seen, pathSeen{path, realPath})
				return nil
			},
		)
		if err != nil {
			t.Fatalf("walk failed: %v", err)
		}
		expected := []pathSeen{
			{path: ".", realPath: "."},
			{path: "symlink_escapes_dir", realPath: ""},
			{path: "symlink_escapes_dir/nested_outside.txt", realPath: ""},
			{path: "file1.txt", realPath: "file1.txt"},
			{path: "symlink_inner_dir", realPath: "subdir"},
			{path: "symlink_inner_dir/symlink_upward_escapes", realPath: ""},
			{path: "symlink_inner_dir/nested_file.txt", realPath: "subdir/nested_file.txt"},
			{path: "symlink_inner_dir/double_nested", realPath: "subdir/double_nested"},
			{path: "symlink_inner_dir/double_nested/double_nested.txt", realPath: "subdir/double_nested/double_nested.txt"},
			{path: "symlink_inner_dir/symlink_upward", realPath: "file1.txt"},
			{path: "symlink_inner", realPath: "file1.txt"},
			{path: "subdir", realPath: "subdir"},
			{path: "subdir/symlink_upward_escapes", realPath: ""},
			{path: "subdir/nested_file.txt", realPath: "subdir/nested_file.txt"},
			{path: "subdir/double_nested", realPath: "subdir/double_nested"},
			{path: "subdir/double_nested/double_nested.txt", realPath: "subdir/double_nested/double_nested.txt"},
			{path: "subdir/symlink_upward", realPath: "file1.txt"},
			{path: "file2.txt", realPath: "file2.txt"},
			{path: "symlink_escapes", realPath: ""},
		}
		assertPathSeen(t, expected, seen)
	})
}

func TestWalk_Rooted_no_loop(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	acceptancetest.MakeOsFsys(tempDir, true, false)
	r, err := osfs.NewRooted(filepath.Join(tempDir, "root", "readable"))
	if err != nil {
		panic(err)
	}
	defer r.Close()

	t.Run("symlink not follow", func(t *testing.T) {
		var seen []pathSeen
		err := vroot.WalkDir(
			r,
			".",
			nil,
			func(path, realPath string, d fs.FileInfo, err error) error {
				if err != nil {
					return err
				}
				seen = append(seen, pathSeen{path, realPath})
				return nil
			},
		)
		if err != nil {
			t.Fatalf("walk failed: %v", err)
		}
		expected := []pathSeen{
			{path: ".", realPath: "."},
			{path: "symlink_escapes_dir", realPath: "symlink_escapes_dir"},
			{path: "file1.txt", realPath: "file1.txt"},
			{path: "symlink_inner_dir", realPath: "symlink_inner_dir"},
			{path: "symlink_inner", realPath: "symlink_inner"},
			{path: "subdir", realPath: "subdir"},
			{path: "subdir/symlink_upward_escapes", realPath: "subdir/symlink_upward_escapes"},
			{path: "subdir/nested_file.txt", realPath: "subdir/nested_file.txt"},
			{path: "subdir/double_nested", realPath: "subdir/double_nested"},
			{path: "subdir/double_nested/double_nested.txt", realPath: "subdir/double_nested/double_nested.txt"},
			{path: "subdir/symlink_upward", realPath: "subdir/symlink_upward"},
			{path: "file2.txt", realPath: "file2.txt"},
			{path: "symlink_escapes", realPath: "symlink_escapes"},
		}
		assertPathSeen(t, expected, seen)
	})
	t.Run("symlink follow", func(t *testing.T) {
		var seen []pathSeen
		err := vroot.WalkDir(
			r,
			".",
			&vroot.WalkOption{ResolveSymlink: true},
			func(path, realPath string, d fs.FileInfo, err error) error {
				if err != nil {
					if errors.Is(err, vroot.ErrPathEscapes) {
						if !slices.Contains([]string{
							filepath.FromSlash("subdir/symlink_upward_escapes"),
							filepath.FromSlash("symlink_inner_dir/symlink_upward_escapes"),
							filepath.FromSlash("symlink_escapes"),
							filepath.FromSlash("symlink_escapes_dir"),
						}, path) {
							t.Errorf("error for path %q is %v but path is not escaping", path, err)
						}
						return nil
					}
					return err
				}
				seen = append(seen, pathSeen{path, realPath})
				return nil
			},
		)
		if err != nil {
			t.Fatalf("walk failed: %v", err)
		}
		expected := []pathSeen{
			{path: ".", realPath: "."},
			{path: "file1.txt", realPath: "file1.txt"},
			{path: "symlink_inner_dir", realPath: "subdir"},
			{path: "symlink_inner_dir/nested_file.txt", realPath: "subdir/nested_file.txt"},
			{path: "symlink_inner_dir/double_nested", realPath: "subdir/double_nested"},
			{path: "symlink_inner_dir/double_nested/double_nested.txt", realPath: "subdir/double_nested/double_nested.txt"},
			{path: "symlink_inner_dir/symlink_upward", realPath: "file1.txt"},
			{path: "symlink_inner", realPath: "file1.txt"},
			{path: "subdir", realPath: "subdir"},
			{path: "subdir/nested_file.txt", realPath: "subdir/nested_file.txt"},
			{path: "subdir/double_nested", realPath: "subdir/double_nested"},
			{path: "subdir/double_nested/double_nested.txt", realPath: "subdir/double_nested/double_nested.txt"},
			{path: "subdir/symlink_upward", realPath: "file1.txt"},
			{path: "file2.txt", realPath: "file2.txt"},
		}
		assertPathSeen(t, expected, seen)
	})
}

func TestWalk_Rooted_symlinks_targetting_each_other(t *testing.T) {
	tempDir := t.TempDir()
	err := testhelper.ExecuteLines(
		tempDir,
		"root/",
		"root/a -> b",
		"root/b -> a",
	)
	if err != nil {
		panic(err)
	}
	r, err := osfs.NewRooted(filepath.Join(tempDir, "root"))
	if err != nil {
		panic(err)
	}
	defer r.Close()
	err = vroot.WalkDir(
		r,
		".",
		&vroot.WalkOption{ResolveSymlink: true},
		func(path, realPath string, d fs.FileInfo, err error) error {
			return err
		},
	)
	if err == nil || !strings.Contains(err.Error(), "too many levels of symbolic links") {
		t.Fatalf("shoud be \"loop detected\" error but is %v", err)
	}
}

type walkTestCase struct {
	name             func() string
	fsysStructure    []string
	expectedPathSeen []pathSeen
}

var walkTestCases = []walkTestCase{
	{
		func() string {
			return "back to parent"
		},
		[]string{
			"root/",
			"root/a/",
			"root/a/b/",
			"root/a/b/c -> ../../a",
			"root/a/b/d/",
			"root/a/b/f/",
		},
		[]pathSeen{
			{path: ".", realPath: "."},
			{path: "a", realPath: "a"},
			{path: "a/b", realPath: "a/b"},
			{path: "a/b/d", realPath: "a/b/d"},
			{path: "a/b/f", realPath: "a/b/f"},
			{path: "a/b/c", realPath: "a"},
		},
	},
	{
		func() string {
			return "indirect loop"
		},
		[]string{
			"root/",
			"root/a/",
			"root/a/b/", "root/a/b/b1/", "root/a/b/b2/",
			"root/a/c/", "root/a/c/c1/", "root/a/c/c2/",
			"root/a/b/b1/l -> ../../c", "root/a/c/c2/l -> ../../b",
		},
		[]pathSeen{
			{path: ".", realPath: "."},
			{path: "a", realPath: "a"},
			{path: "a/b", realPath: "a/b"},
			{path: "a/b/b1", realPath: "a/b/b1"},
			{path: "a/b/b1/l", realPath: "a/c"},
			{path: "a/b/b1/l/c2", realPath: "a/c/c2"},
			{path: "a/b/b1/l/c2/l", realPath: "a/b"},
			{path: "a/b/b1/l/c1", realPath: "a/c/c1"},
			{path: "a/b/b2", realPath: "a/b/b2"},
			{path: "a/c", realPath: "a/c"},
			{path: "a/c/c2", realPath: "a/c/c2"},
			{path: "a/c/c2/l", realPath: "a/b"},
			{path: "a/c/c2/l/b1", realPath: "a/b/b1"},
			{path: "a/c/c2/l/b1/l", realPath: "a/c"},
			{path: "a/c/c2/l/b2", realPath: "a/b/b2"},
			{path: "a/c/c1", realPath: "a/c/c1"},
		},
	},
	{
		func() string {
			return "visited multiple times but no loop"
		},
		[]string{
			"root/",
			"root/a/",
			"root/a/b/",
			"root/a/b/b1/",
			"root/a/b/b2/",
			"root/a/c/",
			"root/a/c/c1 -> ../b",
			"root/a/c/c2 -> ../b",
			"root/a/d/",
			"root/a/d/d1 -> ../c/c2",
		},
		[]pathSeen{
			{path: ".", realPath: "."},
			{path: "a", realPath: "a"},
			{path: "a/b", realPath: "a/b"},
			{path: "a/b/b1", realPath: "a/b/b1"},
			{path: "a/b/b2", realPath: "a/b/b2"},
			{path: "a/d", realPath: "a/d"},
			{path: "a/d/d1", realPath: "a/b"},
			{path: "a/d/d1/b1", realPath: "a/b/b1"},
			{path: "a/d/d1/b2", realPath: "a/b/b2"},
			{path: "a/c", realPath: "a/c"},
			{path: "a/c/c2", realPath: "a/b"},
			{path: "a/c/c2/b1", realPath: "a/b/b1"},
			{path: "a/c/c2/b2", realPath: "a/b/b2"},
			{path: "a/c/c1", realPath: "a/b"},
			{path: "a/c/c1/b1", realPath: "a/b/b1"},
			{path: "a/c/c1/b2", realPath: "a/b/b2"},
		},
	},
}

func TestWalk_Rooted_loop(t *testing.T) {
	for _, tc := range walkTestCases {
		t.Run(tc.name(), func(t *testing.T) {
			tempDir := t.TempDir()
			err := testhelper.ExecuteLines(
				tempDir,
				tc.fsysStructure...,
			)
			if err != nil {
				panic(err)
			}
			r, err := osfs.NewRooted(filepath.Join(tempDir, "root"))
			if err != nil {
				panic(err)
			}
			defer r.Close()
			var seen []pathSeen
			err = vroot.WalkDir(
				r,
				".",
				&vroot.WalkOption{ResolveSymlink: true},
				func(path, realPath string, d fs.FileInfo, err error) error {
					if err != nil {
						return err
					}
					seen = append(seen, pathSeen{path, realPath})
					return err
				},
			)
			if err != nil {
				t.Errorf("WalkDir failed with %v", err)
			}
			assertPathSeen(t, tc.expectedPathSeen, seen)
		})
	}
}

func TestWalk_Unrooted_loop(t *testing.T) {
	outsideCases := []walkTestCase{
		{
			func() string { return "loop outside" },
			[]string{
				"outside/",
				"outside/a/",
				"outside/a/b/",
				"outside/a/b/c -> ../..",
				"root/",
				"root/d -> ../outside",
			},
			[]pathSeen{
				{path: ".", realPath: "."},
				{path: "d", realPath: ""},
				{path: "d/a", realPath: ""},
				{path: "d/a/b", realPath: ""},
				{path: "d/a/b/c", realPath: ""},
			},
		},
		{ // loop is detected and broken but is only by inode
			func() string { return "indirect loop outside" },
			[]string{
				"outside/",
				"outside/a/",
				"outside/a/b/",
				"outside/a/b/b1/",
				"outside/a/b/b2/",
				"outside/a/c/",
				"outside/a/c/c1 -> ../b",
				"outside/a/c/c2 -> ../b",
				"outside/a/d/",
				"outside/a/d/d1 -> ../c/c2",
				"root/",
				"root/a -> ../outside/a",
			},
			[]pathSeen{
				{path: ".", realPath: "."},
				{path: "a", realPath: ""},
				{path: "a/b", realPath: ""},
				{path: "a/b/b1", realPath: ""},
				{path: "a/b/b2", realPath: ""},
				{path: "a/d", realPath: ""},
				{path: "a/d/d1", realPath: ""},
				{path: "a/d/d1/b1", realPath: ""},
				{path: "a/d/d1/b2", realPath: ""},
				{path: "a/c", realPath: ""},
				{path: "a/c/c2", realPath: ""},
				{path: "a/c/c2/b1", realPath: ""},
				{path: "a/c/c2/b2", realPath: ""},
				{path: "a/c/c1", realPath: ""},
				{path: "a/c/c1/b1", realPath: ""},
				{path: "a/c/c1/b2", realPath: ""},
			},
		},
	}
	for _, tc := range slices.Concat(walkTestCases, outsideCases) {
		t.Run(tc.name(), func(t *testing.T) {
			tempDir := t.TempDir()
			err := testhelper.ExecuteLines(
				tempDir,
				tc.fsysStructure...,
			)
			if err != nil {
				panic(err)
			}
			r, err := osfs.NewUnrooted(filepath.Join(tempDir, "root"))
			if err != nil {
				panic(err)
			}
			defer r.Close()
			var seen []pathSeen
			err = vroot.WalkDir(
				r,
				".",
				&vroot.WalkOption{ResolveSymlink: true},
				func(path, realPath string, d fs.FileInfo, err error) error {
					if err != nil {
						return err
					}
					seen = append(seen, pathSeen{path, realPath})
					return err
				},
			)
			if err != nil {
				t.Errorf("WalkDir failed with %v", err)
			}
			assertPathSeen(t, tc.expectedPathSeen, seen)
		})
	}
}
