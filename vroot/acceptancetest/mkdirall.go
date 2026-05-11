package acceptancetest

import (
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestMkdirAll exercises [vroot.Fs.MkdirAll].
//
// MkdirAll creates all intermediate directories. Unlike Mkdir, it returns nil if
// the path already exists as a directory.
func TestMkdirAll[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	t.Run("creates nested directories", func(t *testing.T) {
		c.MkdirAll("a/b/c", 0o755)
		info, err := fsys.Stat("a/b/c")
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("MkdirAll did not produce a directory at the leaf")
		}
		// Intermediates should also be directories.
		for _, p := range []string{"a", "a/b"} {
			info, err := fsys.Stat(p)
			if err != nil {
				t.Errorf("intermediate %q: Stat: %v", p, err)
				continue
			}
			if !info.IsDir() {
				t.Errorf("intermediate %q: not a directory", p)
			}
		}
	})

	t.Run("idempotent on existing directory", func(t *testing.T) {
		c.SetupLines("already/")
		// Calling MkdirAll on an existing directory should be nil.
		if err := fsys.MkdirAll("already", 0o755); err != nil {
			t.Errorf("MkdirAll on existing dir: %v", err)
		}
		if err := fsys.MkdirAll("already", 0o755); err != nil {
			t.Errorf("MkdirAll twice on existing dir: %v", err)
		}
	})

	t.Run("fails when path is a file", func(t *testing.T) {
		c.SetupLines(`afile.txt: "x"`)
		if err := fsys.MkdirAll("afile.txt", 0o755); err == nil {
			t.Errorf("MkdirAll on file path: want error, got nil")
		}
	})

	t.Run("fails when intermediate is a file", func(t *testing.T) {
		c.SetupLines(`stop.txt: "x"`)
		if err := fsys.MkdirAll("stop.txt/below", 0o755); err == nil {
			t.Errorf("MkdirAll with file intermediate: want error, got nil")
		}
	})
}
