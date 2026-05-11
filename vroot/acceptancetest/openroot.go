package acceptancetest

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestOpenRoot exercises [vroot.Root.OpenRoot].
//
// OpenRoot returns a new Root scoped to the given path. The returned Root must continue
// to honor the rooted invariant: paths cannot escape the new root via ".." or symlinks.
func TestOpenRoot[F vroot.File, R vroot.Root[F, R]](t *testing.T, s SetupRoot[F, R]) {
	r := makeRoot(t, s)
	c := newC(t, r)

	c.SetupLines(
		"sub/",
		`sub/inside.txt: "x"`,
	)

	t.Run("opens existing directory", func(t *testing.T) {
		sub, err := r.OpenRoot("sub")
		if err != nil {
			t.Fatalf("OpenRoot: %v", err)
		}
		defer func() { _ = sub.Close() }()

		// File inside the sub-root is accessible from the sub-root.
		f, err := sub.Open("inside.txt")
		if err != nil {
			t.Fatalf("sub.Open: %v", err)
		}
		_ = f.Close()
	})

	t.Run("sub-root forbids escape via dot-dot", func(t *testing.T) {
		sub, err := r.OpenRoot("sub")
		if err != nil {
			t.Fatalf("OpenRoot: %v", err)
		}
		defer func() { _ = sub.Close() }()

		_, err = sub.Open("..")
		if err == nil {
			t.Fatalf("sub.Open(..): want error, got nil")
		}
		if !errors.Is(err, vroot.ErrPathEscapes) {
			t.Errorf("sub.Open(..): want ErrPathEscapes, got %v", err)
		}
	})

	t.Run("non-existent path", func(t *testing.T) {
		sub, err := r.OpenRoot("does-not-exist")
		if err == nil {
			_ = sub.Close()
			t.Fatalf("OpenRoot missing: want error, got nil")
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("OpenRoot missing: want fs.ErrNotExist, got %v", err)
		}
	})

	t.Run("path is a file", func(t *testing.T) {
		c.SetupLines(`afile.txt: "x"`)
		sub, err := r.OpenRoot("afile.txt")
		if err == nil {
			_ = sub.Close()
			t.Fatalf("OpenRoot on file: want error, got nil")
		}
	})
}
