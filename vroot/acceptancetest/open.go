package acceptancetest

import (
	"errors"
	"io"
	"io/fs"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestOpen exercises [vroot.Fs.Open].
//
// Open opens an existing file for reading. The returned file must report Stat
// truthfully and ReadDir must fail when applied to a regular file.
func TestOpen[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(
		"dir/",
		`file.txt: "hello"`,
	)

	t.Run("regular file", func(t *testing.T) {
		f := c.Open("file.txt")
		defer func() { _ = f.Close() }()

		got, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if string(got) != "hello" {
			t.Errorf("content: got %q, want %q", got, "hello")
		}

		// ReadDir / Readdir / Readdirnames on a regular file must error.
		f2 := c.Open("file.txt")
		defer func() { _ = f2.Close() }()
		if _, err := f2.ReadDir(-1); err == nil {
			t.Errorf("ReadDir on regular file: want error, got nil")
		}
	})

	t.Run("directory", func(t *testing.T) {
		f := c.Open("dir")
		defer func() { _ = f.Close() }()

		info, err := f.Stat()
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if !info.IsDir() {
			t.Errorf("opened dir not reported as directory")
		}

		// Read should error on a directory.
		buf := make([]byte, 10)
		if _, err := f.Read(buf); err == nil {
			t.Errorf("Read on directory: want error, got nil")
		}
	})

	t.Run("non-existent path", func(t *testing.T) {
		f, err := fsys.Open("does-not-exist")
		if err == nil {
			_ = f.Close()
			t.Fatalf("Open missing path: want error, got nil")
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Open missing path: want fs.ErrNotExist, got %v", err)
		}
	})

	t.Run("returned file is read-only", func(t *testing.T) {
		f := c.Open("file.txt")
		defer func() { _ = f.Close() }()
		if _, err := f.Write([]byte("x")); err == nil {
			t.Errorf("Write on read-only file: want error, got nil")
		}
	})
}
