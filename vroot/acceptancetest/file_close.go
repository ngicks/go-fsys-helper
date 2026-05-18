package acceptancetest

import (
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileClose exercises [vroot.File.Close].
//
// Close must succeed once. A second Close may return any error but must not panic.
// After Close, Read/Write should return fs.ErrClosed (or wrap it).
func TestFileClose[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s, `f.txt: "abc"`)
	c := newC(t, fsys)

	t.Run("single close succeeds", func(t *testing.T) {
		f := c.Open("f.txt")
		testhelper.NilErr(t, f.Close())
	})

	t.Run("read after close errors with fs.ErrClosed", func(t *testing.T) {
		f := c.Open("f.txt")
		_ = f.Close()
		buf := make([]byte, 3)
		_, err := f.Read(buf)
		testhelper.ErrIs(t, err, fs.ErrClosed)
	})

	t.Run("double close does not panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("double Close panicked: %v", r)
			}
		}()
		f := c.Open("f.txt")
		_ = f.Close()
		_ = f.Close()
	})
}
