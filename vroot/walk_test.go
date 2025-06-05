package vroot_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/internal/prepare"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

type pathSeen struct {
	path     string
	realPath string
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
					if errors.Is(err, vroot.ErrPathEscapes) {
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
		if !slices.Equal(expected, seen) {
			t.Fatalf("not equal:\nexpected: %#v\nactual  :%#v", expected, seen)
		}
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
			{path: "symlink_inner", realPath: "file1.txt"},
			{path: "subdir", realPath: "subdir"},
			{path: "subdir/nested_file.txt", realPath: "subdir/nested_file.txt"},
			{path: "subdir/double_nested", realPath: "subdir/double_nested"},
			{path: "subdir/double_nested/double_nested.txt", realPath: "subdir/double_nested/double_nested.txt"},
			{path: "subdir/symlink_upward", realPath: "file1.txt"},
			{path: "file2.txt", realPath: "file2.txt"},
		}
		if !slices.Equal(expected, seen) {
			t.Fatalf("not equal:\nexpected: %#v\nactual  :%#v", expected, seen)
		}
	})
}
