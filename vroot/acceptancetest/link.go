package acceptancetest

import (
	"io"
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestLink exercises [vroot.Fs.Link] (hard link creation).
func TestLink[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	if s.Option.SkipHardlink {
		t.Skip("SkipHardlink is set")
	}

	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(`src.txt: "content"`)

	t.Run("creates hard link", func(t *testing.T) {
		c.Link("src.txt", "dst.txt")

		// Hard link should be a regular file with the same content.
		r := c.Open("dst.txt")
		defer func() { _ = r.Close() }()

		got, err := io.ReadAll(r)
		testhelper.NilErr(t, err)
		if string(got) != "content" {
			t.Errorf("hard link content: got %q, want %q", got, "content")
		}

		info, err := fsys.Lstat("dst.txt")
		testhelper.NilErr(t, err)
		if info.Mode()&fs.ModeSymlink != 0 {
			t.Errorf("hard link should not have symlink mode, got %s", info.Mode())
		}
	})

	t.Run("writes through hard link reflect on source", func(t *testing.T) {
		c.SetupLines(`through-src.txt: "before"`)
		c.Link("through-src.txt", "through-dst.txt")

		f := c.OpenFile("through-dst.txt", openFlagWriteTrunc(), 0o644)
		_, err := f.Write([]byte("after"))
		testhelper.NilErr(t, err)
		_ = f.Close()

		r := c.Open("through-src.txt")
		defer func() { _ = r.Close() }()

		got, err := io.ReadAll(r)
		testhelper.NilErr(t, err)
		if string(got) != "after" {
			t.Errorf("after writing to hardlink, source content: got %q, want %q", got, "after")
		}
	})

	t.Run("source does not exist", func(t *testing.T) {
		err := fsys.Link("does-not-exist", "x.txt")
		testhelper.ErrIs(t, err, fs.ErrNotExist)
	})

	t.Run("target already exists", func(t *testing.T) {
		c.SetupLines(
			`existing-a.txt: "a"`,
			`existing-b.txt: "b"`,
		)
		err := fsys.Link("existing-a.txt", "existing-b.txt")
		if err == nil {
			t.Errorf("Link to existing target: want error, got nil")
		}
	})
}
