package acceptancetest

import (
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestLchown exercises [vroot.Fs.Lchown].
//
// Lchown should change the ownership of the symlink itself, not its target. Some
// implementations (and platforms) may not distinguish lchown from chown; the test
// only asserts that the call succeeds with the test's uid/gid.
func TestLchown[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	if s.Option.SkipChown {
		t.Skip("SkipChown is set")
	}

	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(`file.txt: "x"`)
	if !s.Option.SkipSymlink {
		c.SetupLines("link -> file.txt")
	}

	t.Run("on file", func(t *testing.T) {
		testhelper.NilErr(t, fsys.Lchown("file.txt", s.Option.ChownUid, s.Option.ChownGid))
	})

	if !s.Option.SkipSymlink {
		t.Run("on symlink", func(t *testing.T) {
			testhelper.NilErr(t, fsys.Lchown("link", s.Option.ChownUid, s.Option.ChownGid))
		})
	}

	t.Run("non-existent path", func(t *testing.T) {
		err := fsys.Lchown("does-not-exist", s.Option.ChownUid, s.Option.ChownGid)
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})
}
