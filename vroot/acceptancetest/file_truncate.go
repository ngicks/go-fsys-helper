package acceptancetest

import (
	"io"
	"os"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileTruncate exercises [vroot.File.Truncate].
func TestFileTruncate[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(`f.txt: "abcdef"`)

	t.Run("shrink", func(t *testing.T) {
		f := c.OpenFile("f.txt", os.O_RDWR, 0)
		defer func() { _ = f.Close() }()

		testhelper.NilErr(t, f.Truncate(3))
		_ = f.Close()

		r := c.Open("f.txt")
		defer func() { _ = r.Close() }()
		got, err := io.ReadAll(r)
		testhelper.NilErr(t, err)
		if string(got) != "abc" {
			t.Errorf("after Truncate(3): got %q, want %q", got, "abc")
		}
	})

	t.Run("extend with zeros", func(t *testing.T) {
		c.SetupLines(`ext.txt: "ab"`)
		f := c.OpenFile("ext.txt", os.O_RDWR, 0)
		defer func() { _ = f.Close() }()

		testhelper.NilErr(t, f.Truncate(5))
		_ = f.Close()

		r := c.Open("ext.txt")
		defer func() { _ = r.Close() }()
		got, err := io.ReadAll(r)
		testhelper.NilErr(t, err)
		if want := "ab\x00\x00\x00"; string(got) != want {
			t.Errorf("after Truncate(5): got %q, want %q", got, want)
		}
	})
}
