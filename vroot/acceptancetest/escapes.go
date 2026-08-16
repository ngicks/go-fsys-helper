package acceptancetest

import (
	"path/filepath"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestRootEscapes asserts that a [vroot.Root] refuses path traversal and symlink escape.
//
// Both syntactic escapes (".." past the root) and symlink-driven escapes (a symlink
// pointing outside the root) must return [vroot.ErrPathEscapes].
func TestRootEscapes[F vroot.File, R vroot.Root[F, R]](t *testing.T, s SetupRoot[F, R]) {
	r := makeRoot(t, s)
	c := newC(t, r)

	c.SetupLines(
		"sub/",
		`sub/inside.txt: "in"`,
	)
	if !s.Option.SkipSymlink {
		c.SetupLines(
			"escapelink -> ../outside",
			"sub/escapelink -> ../../outside",
		)
	}

	traversal := []string{
		"..",
		filepath.FromSlash("../"),
		filepath.FromSlash("../sibling"),
		filepath.FromSlash("sub/../.."),
	}

	for _, p := range traversal {
		t.Run("Open "+p, func(t *testing.T) {
			_, err := r.Open(p)
			testhelper.ErrIs(t, err, vroot.ErrPathEscapes)
		})
		t.Run("Stat "+p, func(t *testing.T) {
			_, err := r.Stat(p)
			testhelper.ErrIs(t, err, vroot.ErrPathEscapes)
		})
		t.Run("Mkdir "+p, func(t *testing.T) {
			err := r.Mkdir(filepath.Join(p, "newdir"), 0o755)
			testhelper.ErrIs(t, err, vroot.ErrPathEscapes)
		})
	}

	if !s.Option.SkipSymlink {
		for _, lnk := range []string{"escapelink", filepath.FromSlash("sub/escapelink")} {
			t.Run("Open via symlink "+lnk, func(t *testing.T) {
				_, err := r.Open(lnk)
				testhelper.ErrIs(t, err, vroot.ErrPathEscapes)
			})
		}
	}
}
