package acceptancetest

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestLstat exercises [vroot.Fs.Lstat].
//
// Lstat returns information about the symlink itself, not its target. This is the
// only way to differentiate symlinks from regular files on Unix.
func TestLstat[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(
		"dir/",
		`file.txt: "x"`,
	)
	if !s.Option.SkipSymlink {
		c.SetupLines("link -> file.txt")
	}

	t.Run("regular file", func(t *testing.T) {
		info, err := fsys.Lstat("file.txt")
		if err != nil {
			t.Fatalf("Lstat: %v", err)
		}
		if info.IsDir() {
			t.Errorf("file reported as directory")
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			t.Errorf("file reported as symlink: mode=%s", info.Mode())
		}
		if info.Size() != 1 {
			t.Errorf("size: got %d, want 1", info.Size())
		}
	})

	t.Run("directory", func(t *testing.T) {
		info, err := fsys.Lstat("dir")
		if err != nil {
			t.Fatalf("Lstat: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("dir not reported as directory")
		}
	})

	if !s.Option.SkipSymlink {
		t.Run("symlink not followed", func(t *testing.T) {
			info, err := fsys.Lstat("link")
			if err != nil {
				t.Fatalf("Lstat: %v", err)
			}
			if info.Mode()&fs.ModeSymlink == 0 {
				t.Errorf("symlink not reported: mode=%s", info.Mode())
			}
		})
	}

	t.Run("non-existent path", func(t *testing.T) {
		_, err := fsys.Lstat("does-not-exist")
		if err == nil {
			t.Fatalf("Lstat on missing path: want error, got nil")
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Lstat on missing path: want fs.ErrNotExist, got %v", err)
		}
	})
}
