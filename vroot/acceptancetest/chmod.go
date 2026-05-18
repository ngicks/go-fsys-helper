package acceptancetest

import (
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestChmod exercises [vroot.Fs.Chmod] on regular files and directories.
//
// Unix: the implementation must change the file's perm bits to the requested value.
// Windows: the implementation must accept the call without error; only the read-only
// bit (0o200) is normally observable but tests do not assert specific bits.
func TestChmod[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	if s.Option.SkipChmod {
		t.Skip("SkipChmod is set")
	}

	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(
		"dir/",
		`file.txt: "hello"`,
	)

	t.Run("on file", func(t *testing.T) {
		var want fs.FileMode
		switch s.Option.Os {
		case OsUnix:
			want = 0o755
		case OsWindows:
			want = 0o666
		}
		c.Chmod("file.txt", want)

		info, err := fsys.Stat("file.txt")
		testhelper.NilErr(t, err)
		switch s.Option.Os {
		case OsUnix:
			if got := info.Mode().Perm(); got != want {
				t.Errorf("file mode after Chmod: got %#o, want %#o", got, want)
			}
		case OsWindows:
			// Windows only respects the read-only bit; we test the inverse pair below.
			if info.IsDir() {
				t.Errorf("file became directory after Chmod")
			}
		}
	})

	t.Run("on directory", func(t *testing.T) {
		var want fs.FileMode
		switch s.Option.Os {
		case OsUnix:
			want = 0o700
		case OsWindows:
			want = 0o555
		}
		c.Chmod("dir", want)

		info, err := fsys.Stat("dir")
		testhelper.NilErr(t, err)
		if !info.IsDir() {
			t.Errorf("dir lost directory mode after Chmod")
		}
		if s.Option.Os == OsUnix {
			if got := info.Mode().Perm(); got != want {
				t.Errorf("dir mode after Chmod: got %#o, want %#o", got, want)
			}
		}
	})

	t.Run("non-existent path", func(t *testing.T) {
		err := fsys.Chmod("does-not-exist", 0o644)
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})

	if s.Option.Os == OsWindows {
		t.Run("readonly toggle", func(t *testing.T) {
			c.Chmod("file.txt", 0o444)
			c.Chmod("file.txt", 0o666)
			f, err := fsys.OpenFile("file.txt", openFlagWrite(), 0)
			testhelper.NilErr(t, err)
			_ = f.Close()
		})
	}
}
