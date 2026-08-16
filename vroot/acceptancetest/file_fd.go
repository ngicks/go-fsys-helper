package acceptancetest

import (
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileFd exercises [vroot.File.Fd].
//
// File implementations not backed by an OS file descriptor must return ^uintptr(0)
// to signal the value is invalid. OS-backed implementations may return any other value.
func TestFileFd[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s, `f.txt: "x"`)
	c := newC(t, fsys)

	f := c.Open("f.txt")
	defer func() { _ = f.Close() }()

	// Just sanity-check that Fd doesn't panic and returns a value.
	_ = f.Fd()
}
