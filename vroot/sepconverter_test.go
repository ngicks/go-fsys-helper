package vroot_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func TestSepConverter(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	acceptancetest.MakeOsFsys(tempDir, true, true)

	t.Run("acceptancetest", func(t *testing.T) {
		r, err := osfs.NewRooted(filepath.Join(tempDir, "root", "writable"))
		if err != nil {
			panic(err)
		}
		defer r.Close()
		osPath := vroot.ToOsPathRooted(r)
		acceptancetest.RootedReadWrite(t, osPath)
	})

	t.Run("accessing with slash separated path", func(t *testing.T) {
		r, err := osfs.NewRooted(filepath.Join(tempDir, "root", "readable"))
		if err != nil {
			panic(err)
		}
		defer r.Close()

		osPath := vroot.ToOsPathRooted(r)

		dirents, err := vroot.ReadDir(osPath, "subdir/")
		if err != nil {
			t.Errorf("readdir failed with %v", err)
		}
		var names []string
		for _, dirent := range dirents {
			names = append(names, dirent.Name())
		}
		slices.Sort(names)
		expected := []string{"double_nested", "nested_file.txt", "symlink_upward", "symlink_upward_escapes"}
		if !slices.Equal(expected, names) {
			t.Errorf("not equal: expected != actual\nexpected:\n%#v\n\nactual:\n%#v", expected, names)
		}
	})
}
