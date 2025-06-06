package vroot_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/internal/prepare"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

type pathSeen struct {
	path     string
	realPath string
}

func assertPathSeen(t *testing.T, expected, actual []pathSeen) {
	if !slices.Equal(expected, actual) {
		t.Fatalf("not equal:\nexpected: %#v\nactual  :%#v", expected, actual)
	}
}

func TestWalk_Rooted_no_loop(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	prepare.MakeFsys(tempDir, true, false)
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
							"subdir/symlink_upward_escapes",
							"symlink_inner_dir/symlink_upward_escapes",
							"symlink_escapes",
							"symlink_escapes_dir",
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
	err := prepare.ExecuteLines(
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

func TestWalk_Rooted_loop(t *testing.T) {
	type testCase struct {
		name             func() string
		fsysStructure    []string
		expectedPathSeen []pathSeen
	}
	testCases := []testCase{
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

	for _, tc := range testCases {
		t.Run(tc.name(), func(t *testing.T) {
			{
				tempDir := t.TempDir()
				err := prepare.ExecuteLines(
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
			}
		})
	}
}
