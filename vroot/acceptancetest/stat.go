package acceptancetest

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestStat exercises [vroot.Fs.Stat].
//
// Stat follows symlinks; Lstat does not.
func TestStat[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(
		"dir/",
		`file.txt: "hello"`,
	)
	if !s.Option.SkipSymlink {
		c.SetupLines("link -> file.txt")
	}

	t.Run("regular file", func(t *testing.T) {
		info, err := fsys.Stat("file.txt")
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if info.IsDir() {
			t.Errorf("file reported as directory")
		}
		if got := info.Size(); got != 5 {
			t.Errorf("size: got %d, want 5", got)
		}
	})

	t.Run("directory", func(t *testing.T) {
		info, err := fsys.Stat("dir")
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("dir not reported as directory")
		}
	})

	if !s.Option.SkipSymlink {
		t.Run("symlink followed", func(t *testing.T) {
			info, err := fsys.Stat("link")
			if err != nil {
				t.Fatalf("Stat: %v", err)
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				t.Errorf("Stat must follow symlink, got mode=%s", info.Mode())
			}
			if info.IsDir() {
				t.Errorf("symlink to file reported as directory")
			}
		})
	}

	t.Run("non-existent path", func(t *testing.T) {
		_, err := fsys.Stat("does-not-exist")
		if err == nil {
			t.Fatalf("Stat missing path: want error, got nil")
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Stat missing path: want fs.ErrNotExist, got %v", err)
		}
	})
}
