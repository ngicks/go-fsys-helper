package overlayfs_test

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestToIoFs walks the merged view as an [io/fs.FS] with [fstest.TestFS], over
// a stack whose entries come from every provenance the overlay has: the top,
// either lower, a name two layers hold at once, one masked by a whiteout and one
// cut off by an opaque directory.
//
// [fstest.TestFS] proves the names it is given are there and that the surface
// around them agrees with itself; the two masked names are asserted separately,
// since a name that is supposed to be gone cannot be expressed as an expectation.
func TestToIoFs(t *testing.T) {
	for _, s := range stacks() {
		t.Run(s.name, func(t *testing.T) {
			f, content := buildStack(t, s)
			defer func() { _ = f.Close() }()

			testhelper.New[*testing.T, vroot.File](t, content[0]).SetupLines(
				`l0only.txt: "l0"`,
				`shadowed.txt: "lower"`,
				"shared/",
				`shared/from0.txt: "from0"`,
			)
			testhelper.New[*testing.T, vroot.File](t, content[1]).SetupLines(
				`gone.txt: "doomed"`,
				"deep/",
				`deep/only.txt: "deep"`,
				"shared/",
				`shared/from1.txt: "from1"`,
				"masked/",
				`masked/hidden.txt: "hidden"`,
			)

			testhelper.New(t, f).SetupLines(
				`shadowed.txt: "top"`,
				"topdir/",
				`topdir/new.txt: "new"`,
			)
			if err := f.Remove("gone.txt"); err != nil {
				t.Fatalf("Remove: %v", err)
			}
			// Removed and re-made rather than made outright: Mkdir refuses a name a
			// lower still shows, and it is the re-made directory that is opaque.
			if err := f.RemoveAll("masked"); err != nil {
				t.Fatalf("RemoveAll: %v", err)
			}
			testhelper.New(t, f).SetupLines(
				"masked/",
				`masked/visible.txt: "visible"`,
			)

			iofs := vroot.ToIoFs[vroot.File](f)
			if err := fstest.TestFS(iofs,
				"l0only.txt",
				"shadowed.txt",
				"shared/from0.txt",
				"shared/from1.txt",
				"deep/only.txt",
				"topdir/new.txt",
				"masked/visible.txt",
			); err != nil {
				t.Fatal(err)
			}

			for _, name := range []string{"gone.txt", "masked/hidden.txt"} {
				if _, err := fs.Stat(iofs, name); !errors.Is(err, fs.ErrNotExist) {
					t.Errorf("stat %q = %v, want a not-exist error", name, err)
				}
			}

			got, err := fs.ReadFile(iofs, "shadowed.txt")
			if err != nil {
				t.Fatalf("readfile: %v", err)
			}
			if string(got) != "top" {
				t.Errorf("shadowed.txt = %q, want %q", got, "top")
			}
		})
	}
}
