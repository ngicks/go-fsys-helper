package acceptancetest

import (
	"io"
	"io/fs"
	"os"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestOpenFile exercises [vroot.Fs.OpenFile] with various flag combinations.
func TestOpenFile[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(`existing.txt: "old"`)

	t.Run("O_RDONLY", func(t *testing.T) {
		f := c.OpenFile("existing.txt", os.O_RDONLY, 0)
		defer func() { _ = f.Close() }()
		got, err := io.ReadAll(f)
		testhelper.NilErr(t, err)
		if string(got) != "old" {
			t.Errorf("content: got %q, want %q", got, "old")
		}
		if _, err := f.Write([]byte("x")); err == nil {
			t.Errorf("Write on O_RDONLY: want error, got nil")
		}
	})

	t.Run("O_WRONLY|O_TRUNC", func(t *testing.T) {
		f := c.OpenFile("existing.txt", os.O_WRONLY|os.O_TRUNC, 0)
		_, err := f.Write([]byte("new"))
		testhelper.NilErr(t, err)
		_ = f.Close()

		r := c.Open("existing.txt")
		defer func() { _ = r.Close() }()
		got, err := io.ReadAll(r)
		testhelper.NilErr(t, err)
		if string(got) != "new" {
			t.Errorf("content after O_TRUNC: got %q, want %q", got, "new")
		}
	})

	t.Run("O_WRONLY|O_APPEND", func(t *testing.T) {
		c.SetupLines(`append.txt: "ab"`)
		f := c.OpenFile("append.txt", os.O_WRONLY|os.O_APPEND, 0)
		_, err := f.Write([]byte("cd"))
		testhelper.NilErr(t, err)
		_ = f.Close()

		r := c.Open("append.txt")
		defer func() { _ = r.Close() }()
		got, err := io.ReadAll(r)
		testhelper.NilErr(t, err)
		if string(got) != "abcd" {
			t.Errorf("content after O_APPEND: got %q, want %q", got, "abcd")
		}
	})

	t.Run("O_CREATE on new path", func(t *testing.T) {
		f := c.OpenFile("created.txt", os.O_WRONLY|os.O_CREATE, 0o644)
		_ = f.Close()
		_, err := fsys.Stat("created.txt")
		testhelper.NilErr(t, err)
	})

	t.Run("O_CREATE|O_EXCL fails when path exists", func(t *testing.T) {
		c.SetupLines(`excl.txt: "x"`)
		_, err := fsys.OpenFile("excl.txt", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		testhelper.ErrIs(t, err, fs.ErrExist)
	})

	t.Run("non-existent path without O_CREATE", func(t *testing.T) {
		_, err := fsys.OpenFile("missing.txt", os.O_RDONLY, 0)
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})

	t.Run("parent does not exist", func(t *testing.T) {
		_, err := fsys.OpenFile("missing-parent/file.txt", os.O_WRONLY|os.O_CREATE, 0o644)
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})
}
