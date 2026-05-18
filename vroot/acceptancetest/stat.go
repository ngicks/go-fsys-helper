package acceptancetest

import (
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestStat exercises [vroot.Fs.Stat].
//
// Stat follows symlinks; Lstat does not.
func TestStat[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	lines := []string{
		"dir/",
		`file.txt: "hello"`,
	}
	if !s.Option.SkipSymlink {
		lines = append(lines, "link -> file.txt")
	}
	fsys := makeFs(t, s, lines...)

	t.Run("regular file", func(t *testing.T) {
		info, err := fsys.Stat("file.txt")
		testhelper.NilErr(t, err)
		if info.IsDir() {
			t.Errorf("file reported as directory")
		}
		if got := info.Size(); got != 5 {
			t.Errorf("size: got %d, want 5", got)
		}
	})

	t.Run("directory", func(t *testing.T) {
		info, err := fsys.Stat("dir")
		testhelper.NilErr(t, err)
		if !info.IsDir() {
			t.Errorf("dir not reported as directory")
		}
	})

	if !s.Option.SkipSymlink {
		t.Run("symlink followed", func(t *testing.T) {
			info, err := fsys.Stat("link")
			testhelper.NilErr(t, err)
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
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})
}
