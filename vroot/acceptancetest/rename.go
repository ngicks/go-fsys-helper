package acceptancetest

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestRename exercises [vroot.Fs.Rename].
func TestRename[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	if s.Option.SkipRename {
		t.Skip("SkipRename is set")
	}

	fsys := makeFs(t, s)
	c := newC(t, fsys)

	t.Run("file", func(t *testing.T) {
		c.SetupLines(`old.txt: "x"`)
		c.Rename("old.txt", "new.txt")
		if _, err := fsys.Stat("old.txt"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("after Rename, old path Stat: want fs.ErrNotExist, got %v", err)
		}
		if _, err := fsys.Stat("new.txt"); err != nil {
			t.Errorf("after Rename, new path Stat: %v", err)
		}
	})

	t.Run("directory", func(t *testing.T) {
		c.SetupLines(
			"olddir/",
			`olddir/inside.txt: "x"`,
		)
		c.Rename("olddir", "newdir")
		if _, err := fsys.Stat("olddir"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("after Rename, old path Stat: want fs.ErrNotExist, got %v", err)
		}
		if _, err := fsys.Stat("newdir/inside.txt"); err != nil {
			t.Errorf("after Rename, inside Stat: %v", err)
		}
	})

	t.Run("source does not exist", func(t *testing.T) {
		err := fsys.Rename("missing", "anything")
		if err == nil {
			t.Fatalf("Rename missing: want error, got nil")
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Rename missing: want fs.ErrNotExist, got %v", err)
		}
	})
}
