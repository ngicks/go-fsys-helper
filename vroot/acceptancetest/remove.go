package acceptancetest

import (
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestRemove exercises [vroot.Fs.Remove].
//
// Remove deletes a file or empty directory. Non-empty directories must error.
func TestRemove[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	t.Run("file", func(t *testing.T) {
		c.SetupLines(`f.txt: "x"`)
		c.Remove("f.txt")
		_, err := fsys.Stat("f.txt")
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})

	t.Run("empty directory", func(t *testing.T) {
		c.SetupLines("emptydir/")
		c.Remove("emptydir")
		_, err := fsys.Stat("emptydir")
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})

	if !s.Option.SkipSymlink {
		t.Run("symlink itself, not the target", func(t *testing.T) {
			c.SetupLines(
				`linktarget.txt: "y"`,
				"symremove -> linktarget.txt",
			)
			c.Remove("symremove")
			_, err := fsys.Lstat("symremove")
			testhelper.ErrIs(t, err, fs.ErrNotExist)
			// Target should still exist.
			_, err = fsys.Stat("linktarget.txt")
			testhelper.NilErr(t, err)
		})
	}

	t.Run("non-empty directory errors", func(t *testing.T) {
		c.SetupLines(
			"nonempty/",
			`nonempty/inside.txt: "x"`,
		)
		err := fsys.Remove("nonempty")
		if err == nil {
			t.Errorf("Remove on non-empty directory: want error, got nil")
		}
	})

	t.Run("non-existent path", func(t *testing.T) {
		err := fsys.Remove("does-not-exist")
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})
}
