package acceptancetest

import (
	"errors"
	"io/fs"
	"syscall"
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

	// A path whose final element is "." must be refused with EINVAL, matching
	// os.RemoveAll / *os.Root.RemoveAll (rmdir(2) cannot remove "."). The tree
	// must survive. Path-normalizing wrappers that filepath.Clean before
	// delegating cannot observe the trailing dot and set SkipRemoveAllDotComponent.
	if !s.Option.SkipRemoveAllDotComponent {
		t.Run("last component is dot returns EINVAL", func(t *testing.T) {
			c.SetupLines(
				"keep/",
				`keep/leaf.txt: "x"`,
			)
			for _, name := range []string{".", "keep/."} {
				err := fsys.RemoveAll(name)
				if !errors.Is(err, syscall.EINVAL) {
					t.Errorf("RemoveAll(%q): want errors.Is EINVAL, got %v", name, err)
				}
			}
			if _, err := fsys.Stat("keep"); err != nil {
				t.Errorf("tree removed despite dot-component RemoveAll: %v", err)
			}
		})
	}
}
