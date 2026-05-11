package acceptancetest

import (
	"errors"
	"io"
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestCreate exercises [vroot.Fs.Create].
//
// Create creates a new file with mode 0666 (subject to umask), truncating an existing file.
// It must not create intermediate directories.
func TestCreate[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(
		"existing/",
		`existing.txt: "old content"`,
	)

	t.Run("new file", func(t *testing.T) {
		f := c.Create("new.txt")
		defer func() { _ = f.Close() }()

		n, err := f.Write([]byte("hello"))
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
		if n != 5 {
			t.Errorf("Write returned n=%d, want 5", n)
		}
	})

	t.Run("truncates existing file", func(t *testing.T) {
		f := c.Create("existing.txt")
		defer func() { _ = f.Close() }()

		// We re-open and read instead of relying on Seek so this works on Fs that
		// return ErrOpNotSupported for Seek.
		_ = f.Close()
		r := c.Open("existing.txt")
		defer func() { _ = r.Close() }()

		got, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("after Create, file should be truncated; got %q", got)
		}
	})

	t.Run("parent does not exist", func(t *testing.T) {
		f, err := fsys.Create("missing-dir/file.txt")
		if err == nil {
			_ = f.Close()
			t.Fatalf("Create with missing parent: want error, got nil")
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Create with missing parent: want fs.ErrNotExist, got %v", err)
		}
	})

	t.Run("rejects target is directory", func(t *testing.T) {
		f, err := fsys.Create("existing")
		if err == nil {
			_ = f.Close()
			t.Fatalf("Create on directory path: want error, got nil")
		}
	})
}
