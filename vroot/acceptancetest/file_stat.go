package acceptancetest

import (
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileStat exercises [vroot.File.Stat].
func TestFileStat[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s, `f.txt: "hello"`)
	c := newC(t, fsys)

	f := c.Open("f.txt")
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	testhelper.NilErr(t, err)
	if info.IsDir() {
		t.Errorf("file reported as directory")
	}
	if got := info.Size(); got != 5 {
		t.Errorf("size: got %d, want 5", got)
	}
}
