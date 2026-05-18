package acceptancetest

import (
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestRemoveAll exercises [vroot.Fs.RemoveAll].
//
// RemoveAll recursively deletes a path tree. It returns nil if the path does not exist.
func TestRemoveAll[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	t.Run("file", func(t *testing.T) {
		c.SetupLines(`one.txt: "x"`)
		c.RemoveAll("one.txt")
		_, err := fsys.Stat("one.txt")
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})

	t.Run("nested tree", func(t *testing.T) {
		c.SetupLines(
			"tree/",
			"tree/a/",
			"tree/a/b/",
			`tree/a/b/leaf.txt: "leaf"`,
			`tree/a/sibling.txt: "x"`,
		)
		c.RemoveAll("tree")
		_, err := fsys.Stat("tree")
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})

	t.Run("idempotent on missing path", func(t *testing.T) {
		testhelper.NilErr(t, fsys.RemoveAll("never-existed"))
	})
}
