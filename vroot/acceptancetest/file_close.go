package acceptancetest

import (
	"errors"
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileClose exercises [vroot.File.Close].
//
// Close must succeed once. A second Close may return any error but must not panic.
// After Close, Read/Write should return fs.ErrClosed (or wrap it).
func TestFileClose[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(`f.txt: "abc"`)

	t.Run("single close succeeds", func(t *testing.T) {
		f := c.Open("f.txt")
		if err := f.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	t.Run("read after close errors with fs.ErrClosed", func(t *testing.T) {
		f := c.Open("f.txt")
		_ = f.Close()
		buf := make([]byte, 3)
		_, err := f.Read(buf)
		if err == nil {
			t.Errorf("Read after Close: want error, got nil")
			return
		}
		if !errors.Is(err, fs.ErrClosed) {
			t.Errorf("Read after Close: want fs.ErrClosed, got %v", err)
		}
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
