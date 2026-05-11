package acceptancetest

import (
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestChtimes exercises [vroot.Fs.Chtimes].
func TestChtimes[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	if s.Option.SkipChtimes {
		t.Skip("SkipChtimes is set")
	}

	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(`file.txt: "x"`)

	// Use values an hour in the past so we don't accidentally match the freshly created mtime.
	atime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	mtime := time.Date(2025, 6, 7, 8, 9, 10, 0, time.UTC)

	t.Run("set mtime", func(t *testing.T) {
		c.Chtimes("file.txt", atime, mtime)
		info, err := fsys.Stat("file.txt")
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		// allow up to 1 second slack for filesystems with low resolution timestamps.
		if diff := info.ModTime().Sub(mtime).Abs(); diff > time.Second {
			t.Errorf("modtime: got %v, want %v (diff %v)", info.ModTime(), mtime, diff)
		}
	})

	t.Run("non-existent path", func(t *testing.T) {
		err := fsys.Chtimes("does-not-exist", atime, mtime)
		if err == nil {
			t.Fatalf("Chtimes on missing file: want error, got nil")
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Chtimes on missing file: want fs.ErrNotExist, got %v", err)
		}
	})
}
