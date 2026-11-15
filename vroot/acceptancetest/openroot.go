package acceptancetest

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestOpenRoot exercises [vroot.Root.OpenRoot].
//
// OpenRoot returns a new Root scoped to the given path. The returned Root must continue
// to honor the rooted invariant: paths cannot escape the new root via ".." or symlinks.
func TestOpenRoot[F vroot.File, R vroot.Root[F, R]](t *testing.T, s SetupRoot[F, R]) {
	r := makeRoot(t, s,
		"sub/",
		`sub/inside.txt: "x"`,
		`afile.txt: "x"`,
	)

	t.Run("opens existing directory", func(t *testing.T) {
		sub, err := r.OpenRoot("sub")
		testhelper.NilErr(t, err)
		defer func() { _ = sub.Close() }()

		// File inside the sub-root is accessible from the sub-root.
		f, err := sub.Open("inside.txt")
		testhelper.NilErr(t, err)
		_ = f.Close()
	})

	rootNames := func(t *testing.T, root R) []string {
		t.Helper()
		info, err := root.Stat(".")
		testhelper.NilErr(t, err)
		if !info.IsDir() {
			t.Errorf("root not reported as directory")
		}

		f, err := root.Open(".")
		testhelper.NilErr(t, err)
		defer func() { _ = f.Close() }()
		names, err := f.Readdirnames(-1)
		testhelper.NilErr(t, err)
		slices.Sort(names)
		return names
	}

	t.Run("sub-root's own root lists its content", func(t *testing.T) {
		sub, err := r.OpenRoot("sub")
		testhelper.NilErr(t, err)
		defer func() { _ = sub.Close() }()

		if got, want := rootNames(t, sub), []string{"inside.txt"}; !slices.Equal(got, want) {
			t.Errorf("sub-root names: got %v, want %v", got, want)
		}
	})

	t.Run("root opened as sub-root lists the root", func(t *testing.T) {
		sub, err := r.OpenRoot(".")
		testhelper.NilErr(t, err)
		defer func() { _ = sub.Close() }()

		if got, want := rootNames(t, sub), []string{"afile.txt", "sub"}; !slices.Equal(got, want) {
			t.Errorf("sub-root names: got %v, want %v", got, want)
		}
	})

	t.Run("sub-root forbids escape via dot-dot", func(t *testing.T) {
		sub, err := r.OpenRoot("sub")
		testhelper.NilErr(t, err)
		defer func() { _ = sub.Close() }()

		_, err = sub.Open("..")
		testhelper.ErrIs(t, err, vroot.ErrPathEscapes)
	})

	t.Run("non-existent path", func(t *testing.T) {
		_, err := r.OpenRoot("does-not-exist")
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})

	t.Run("path is a file", func(t *testing.T) {
		sub, err := r.OpenRoot("afile.txt")
		if err == nil {
			_ = sub.Close()
			t.Fatalf("OpenRoot on file: want error, got nil")
		}
	})
}
