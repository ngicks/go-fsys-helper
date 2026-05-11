package acceptancetest

import (
	"errors"
	"io/fs"
	"testing"

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
		if err := fsys.Lchown("file.txt", s.Option.ChownUid, s.Option.ChownGid); err != nil {
			t.Fatalf("Lchown: %v", err)
		}
	})

	if !s.Option.SkipSymlink {
		t.Run("on symlink", func(t *testing.T) {
			if err := fsys.Lchown("link", s.Option.ChownUid, s.Option.ChownGid); err != nil {
				t.Fatalf("Lchown(symlink): %v", err)
			}
		})
	}

	t.Run("non-existent path", func(t *testing.T) {
		err := fsys.Lchown("does-not-exist", s.Option.ChownUid, s.Option.ChownGid)
		if err == nil {
			t.Fatalf("Lchown on missing file: want error, got nil")
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Lchown on missing file: want fs.ErrNotExist, got %v", err)
		}
	})
}
