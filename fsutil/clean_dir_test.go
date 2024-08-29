package fsutil

import (
	"io/fs"
	"os"
	"testing"

	"github.com/spf13/afero"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

func TestCleanDir(t *testing.T) {
	var err error

	mem := afero.NewMemMapFs()
	err = CopyFS(mem, os.DirFS("testdata"))
	assert.NilError(t, err)

	assertSeen(
		t,
		mem,
		".",
		[]string{
			"random1.txt",
			"fs4",
			"fs4/random1.txt",
			"fs4/random2.txt",
			"fs3",
			"fs3/random1.txt",
			"fs3/random2_just_renamed.txt",
			"fs1",
			"fs1/random1.txt",
			"fs1/random2.txt",
			"fs5",
			"fs5/random1.txt",
			"fs5/random1.txt/.gitkeep",
			"fs5/random2.txt",
			"fs2",
			"fs2/random1.txt",
			"fs2/random2.txt",
			"fs7",
			"fs7/random1.txt",
			"fs7/random2.txt",
			"fs7/additional",
			"fs7/additional/.gitkeep",
			"fs8",
			"fs8/random1.txt",
			"fs8/random2.txt",
			"fs6",
			"fs6/random1.txt",
			"fs6/random1.txt/.gitkeep",
			"fs6/random2.txt",
			"random2.txt",
		},
	)

	assertSeen(t, mem, "fs1", []string{"fs1/random1.txt", "fs1/random2.txt"})
	err = CleanDir(mem, "fs1")
	assert.NilError(t, err)
	assertSeen(t, mem, "fs1", []string{})

	assertSeen(t, mem, "fs6", []string{"fs6/random1.txt", "fs6/random1.txt/.gitkeep", "fs6/random2.txt"})
	err = CleanDir(mem, "fs6")
	assert.NilError(t, err)
	assertSeen(t, mem, "fs6", []string{})

	err = CleanDir(mem, ".")
	assert.NilError(t, err)
	assertSeen(t, mem, ".", []string{})
}

func assertSeen(t *testing.T, fsys afero.Fs, root string, paths []string) {
	t.Helper()

	seen := map[string]struct{}{}
	err := fs.WalkDir(afero.NewIOFS(fsys), root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		seen[path] = struct{}{}
		return nil
	})
	assert.NilError(t, err)

	pathsMap := map[string]struct{}{}
	for _, p := range paths {
		pathsMap[p] = struct{}{}
	}

	assert.Assert(t, cmp.DeepEqual(seen, pathsMap))
}
