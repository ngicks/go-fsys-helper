package acceptancetest

import (
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestChown exercises [vroot.Fs.Chown] on regular files and directories.
//
// Chown is permitted to be a no-op or return an error on systems where the test process
// lacks privileges. When Option.SkipChown is set this test only asserts that calling
// Chown with the test's uid/gid does not return an error.
func TestChown[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	if s.Option.SkipChown {
		t.Skip("SkipChown is set")
	}

	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(
		"dir/",
		`file.txt: "x"`,
	)

	t.Run("on file", func(t *testing.T) {
		testhelper.NilErr(t, fsys.Chown("file.txt", s.Option.ChownUid, s.Option.ChownGid))
	})

	t.Run("on directory", func(t *testing.T) {
		testhelper.NilErr(t, fsys.Chown("dir", s.Option.ChownUid, s.Option.ChownGid))
	})

	t.Run("non-existent path", func(t *testing.T) {
		err := fsys.Chown("does-not-exist", s.Option.ChownUid, s.Option.ChownGid)
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})
}
