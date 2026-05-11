package acceptancetest

import (
	"errors"
	"path/filepath"
	"testing"

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
			f, err := r.Open(p)
			if err == nil {
				_ = f.Close()
				t.Fatalf("Open(%q): want error, got nil", p)
			}
			if !errors.Is(err, vroot.ErrPathEscapes) {
				t.Errorf("Open(%q): want ErrPathEscapes, got %v", p, err)
			}
		})
		t.Run("Stat "+p, func(t *testing.T) {
			if _, err := r.Stat(p); !errors.Is(err, vroot.ErrPathEscapes) {
				t.Errorf("Stat(%q): want ErrPathEscapes, got %v", p, err)
			}
		})
		t.Run("Mkdir "+p, func(t *testing.T) {
			err := r.Mkdir(filepath.Join(p, "newdir"), 0o755)
			if !errors.Is(err, vroot.ErrPathEscapes) {
				t.Errorf("Mkdir(%q): want ErrPathEscapes, got %v", filepath.Join(p, "newdir"), err)
			}
		})
	}

	if !s.Option.SkipSymlink {
		for _, lnk := range []string{"escapelink", filepath.FromSlash("sub/escapelink")} {
			t.Run("Open via symlink "+lnk, func(t *testing.T) {
				f, err := r.Open(lnk)
				if err == nil {
					_ = f.Close()
					t.Fatalf("Open(%q): want error, got nil", lnk)
				}
				if !errors.Is(err, vroot.ErrPathEscapes) {
					t.Errorf("Open(%q): want ErrPathEscapes, got %v", lnk, err)
				}
			})
		}
	}
}
