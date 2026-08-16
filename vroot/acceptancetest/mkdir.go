package acceptancetest

import (
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestMkdir exercises [vroot.Fs.Mkdir].
//
// Mkdir creates a single directory. It does NOT create intermediate directories.
// On a path that already exists Mkdir returns ErrExist.
func TestMkdir[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	t.Run("basic", func(t *testing.T) {
		c.Mkdir("d1", 0o755)
		info, err := fsys.Stat("d1")
		testhelper.NilErr(t, err)
		if !info.IsDir() {
			t.Errorf("Mkdir did not produce a directory")
		}
	})

	t.Run("nested when parent exists", func(t *testing.T) {
		c.Mkdir("d2", 0o755)
		c.Mkdir("d2/inner", 0o755)
		info, err := fsys.Stat("d2/inner")
		testhelper.NilErr(t, err)
		if !info.IsDir() {
			t.Errorf("nested mkdir produced non-directory")
		}
	})

	t.Run("fails when parent missing", func(t *testing.T) {
		err := fsys.Mkdir("missing-parent/child", 0o755)
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})

	t.Run("fails when path already exists", func(t *testing.T) {
		c.SetupLines("already/")
		err := fsys.Mkdir("already", 0o755)
		testhelper.ErrIs(t, err, fs.ErrExist)
	})
}
