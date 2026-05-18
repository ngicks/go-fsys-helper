package acceptancetest

import (
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileChmod exercises [vroot.File.Chmod].
func TestFileChmod[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	if s.Option.SkipChmod {
		t.Skip("SkipChmod is set")
	}

	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(`f.txt: "x"`)

	f := c.Open("f.txt")
	defer func() { _ = f.Close() }()

	var want fs.FileMode
	switch s.Option.Os {
	case OsUnix:
		want = 0o600
	case OsWindows:
		want = 0o444
	}

	testhelper.NilErr(t, f.Chmod(want))

	info, err := fsys.Stat("f.txt")
	testhelper.NilErr(t, err)
	if s.Option.Os == OsUnix {
		if got := info.Mode().Perm(); got != want {
			t.Errorf("after File.Chmod, mode: got %#o, want %#o", got, want)
		}
	}
}
