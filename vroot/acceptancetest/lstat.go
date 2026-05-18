package acceptancetest

import (
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestLstat exercises [vroot.Fs.Lstat].
//
// Lstat returns information about the symlink itself, not its target. This is the
// only way to differentiate symlinks from regular files on Unix.
func TestLstat[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	lines := []string{
		"dir/",
		`file.txt: "x"`,
	}
	if !s.Option.SkipSymlink {
		lines = append(lines, "link -> file.txt")
	}
	fsys := makeFs(t, s, lines...)

	t.Run("regular file", func(t *testing.T) {
		info, err := fsys.Lstat("file.txt")
		testhelper.NilErr(t, err)
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
		testhelper.NilErr(t, err)
		if !info.IsDir() {
			t.Errorf("dir not reported as directory")
		}
	})

	if !s.Option.SkipSymlink {
		t.Run("symlink not followed", func(t *testing.T) {
			info, err := fsys.Lstat("link")
			testhelper.NilErr(t, err)
			if info.Mode()&fs.ModeSymlink == 0 {
				t.Errorf("symlink not reported: mode=%s", info.Mode())
			}
		})
	}

	t.Run("non-existent path", func(t *testing.T) {
		_, err := fsys.Lstat("does-not-exist")
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})
}
