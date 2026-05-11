package acceptancetest

import (
	"errors"
	"io/fs"
	"testing"

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
		if _, err := fsys.Stat("one.txt"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("after RemoveAll, Stat: want fs.ErrNotExist, got %v", err)
		}
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
		if _, err := fsys.Stat("tree"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("after RemoveAll, Stat: want fs.ErrNotExist, got %v", err)
		}
	})

	t.Run("idempotent on missing path", func(t *testing.T) {
		if err := fsys.RemoveAll("never-existed"); err != nil {
			t.Errorf("RemoveAll on missing path: want nil, got %v", err)
		}
	})
}
