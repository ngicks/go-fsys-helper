package example

import (
	"io/fs"
	"os"
	"testing"

	"github.com/ngicks/go-fsys-helper/storage"
	"github.com/spf13/afero"
	"gotest.tools/v3/assert"
)

func TestExample(t *testing.T) {
	var (
		set = PathSet{
			Foo: "./Foo",
			Bar: "./Bar",
			Baz: "./Baz",
		}
		handle  PathHandle
		content PathContents = PathContents{
			Foo: os.DirFS("testdata/foo"),
			Baz: os.DirFS("testdata/baz"),
		}
	)

	assert.NilError(t, storage.ValidatePrepareInput(set, &handle))
	assert.NilError(t, storage.ValidateCopyContentsInput(handle, content, true))

	base := afero.NewMemMapFs()

	handle, err := PreparePath(base, set, content)
	assert.NilError(t, err)

	assertSeenPaths(t, handle.Foo, map[string]struct{}{"a": {}})
	assertSeenPaths(t, handle.Bar, map[string]struct{}{})
	assertSeenPaths(t, handle.Baz, map[string]struct{}{"b": {}})
}

func assertSeenPaths(t *testing.T, fsys afero.Fs, seenPaths map[string]struct{}) {
	seen := make([]string, 0)
	err := afero.Walk(fsys, ".", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == "." || info.IsDir() {
			return nil
		}

		seen = append(seen, path)

		_, ok := seenPaths[path]
		assert.Assert(t, ok, "a path was seen but input does not have it, path = %s", path)
		return nil
	})
	assert.NilError(t, err)
	assert.Assert(t, len(seenPaths) == len(seen), "expected = %#v, actual = %#v", seenPaths, seen)
}
