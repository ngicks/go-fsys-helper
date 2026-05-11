package acceptancetest

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestMkdir exercises [vroot.Fs.Mkdir].
//
// Mkdir creates a single directory. It does NOT create intermediate directories.
// On a path that already exists Mkdir returns ErrExist.
func TestMkdir[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	t.Run("basic", func(t *testing.T) {
		c.Mkdir("d1", 0o755)
		info, err := fsys.Stat("d1")
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("Mkdir did not produce a directory")
		}
	})

	t.Run("nested when parent exists", func(t *testing.T) {
		c.Mkdir("d2", 0o755)
		c.Mkdir("d2/inner", 0o755)
		info, err := fsys.Stat("d2/inner")
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("nested mkdir produced non-directory")
		}
	})

	t.Run("fails when parent missing", func(t *testing.T) {
		err := fsys.Mkdir("missing-parent/child", 0o755)
		if err == nil {
			t.Fatalf("Mkdir with missing parent: want error, got nil")
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Mkdir with missing parent: want fs.ErrNotExist, got %v", err)
		}
	})

	t.Run("fails when path already exists", func(t *testing.T) {
		c.SetupLines("already/")
		err := fsys.Mkdir("already", 0o755)
		if err == nil {
			t.Fatalf("Mkdir for existing path: want error, got nil")
		}
		if !errors.Is(err, fs.ErrExist) {
			t.Errorf("Mkdir for existing path: want fs.ErrExist, got %v", err)
		}
	})
}
