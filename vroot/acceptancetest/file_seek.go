package acceptancetest

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileSeek exercises [vroot.File.Seek].
//
// When Option.SkipSeek is set, the implementation may return [vroot.ErrOpNotSupported].
func TestFileSeek[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s, `f.txt: "abcdef"`)
	c := newC(t, fsys)

	f := c.Open("f.txt")
	defer func() { _ = f.Close() }()

	if s.Option.SkipSeek {
		_, err := f.Seek(0, io.SeekStart)
		if err != nil && !errors.Is(err, vroot.ErrOpNotSupported) {
			t.Errorf("Seek on unsupported file: want ErrOpNotSupported or success, got %v", err)
		}
		return
	}

	t.Run("SeekStart", func(t *testing.T) {
		off, err := f.Seek(2, io.SeekStart)
		testhelper.NilErr(t, err)
		if off != 2 {
			t.Errorf("offset: got %d, want 2", off)
		}
		buf := make([]byte, 2)
		_, err = io.ReadFull(f, buf)
		testhelper.NilErr(t, err)
		if !bytes.Equal(buf, []byte("cd")) {
			t.Errorf("read: got %q, want %q", buf, "cd")
		}
	})

	t.Run("SeekCurrent", func(t *testing.T) {
		// We're positioned after the "cd" read above (offset=4).
		off, err := f.Seek(-1, io.SeekCurrent)
		testhelper.NilErr(t, err)
		if off != 3 {
			t.Errorf("offset: got %d, want 3", off)
		}
		buf := make([]byte, 1)
		_, err = io.ReadFull(f, buf)
		testhelper.NilErr(t, err)
		if string(buf) != "d" {
			t.Errorf("read: got %q, want %q", buf, "d")
		}
	})

	t.Run("SeekEnd", func(t *testing.T) {
		off, err := f.Seek(-1, io.SeekEnd)
		testhelper.NilErr(t, err)
		if off != 5 {
			t.Errorf("offset: got %d, want 5", off)
		}
		buf := make([]byte, 1)
		_, err = io.ReadFull(f, buf)
		testhelper.NilErr(t, err)
		if string(buf) != "f" {
			t.Errorf("read: got %q, want %q", buf, "f")
		}
	})
}
