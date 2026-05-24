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

	// Consistency with os.RemoveAll, RemoveAll must return when trying to
	// remove paths with ending `.` component, e.g. `./foo/.` or `.`.
	// However actually test below is relaxed to just only do it against `.`
	// because some wrappers with filepath.Clean may remove ending dot before it
	// reaches actual implementations.
	t.Run("removing root returns EINVAL", func(t *testing.T) {
		c.SetupLines(
			"keep/",
			`keep/leaf.txt: "x"`,
		)
		if err := fsys.RemoveAll("."); !errors.Is(err, syscall.EINVAL) {
			t.Errorf(`RemoveAll("."): want errors.Is EINVAL, got %v`, err)
		}
		if _, err := fsys.Stat("keep"); err != nil {
			t.Errorf("tree removed despite RemoveAll(\".\"): %v", err)
		}
	})
}
