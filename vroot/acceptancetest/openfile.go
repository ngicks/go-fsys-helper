package acceptancetest

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"testing"

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
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if string(got) != "old" {
			t.Errorf("content: got %q, want %q", got, "old")
		}
		if _, err := f.Write([]byte("x")); err == nil {
			t.Errorf("Write on O_RDONLY: want error, got nil")
		}
	})

	t.Run("O_WRONLY|O_TRUNC", func(t *testing.T) {
		f := c.OpenFile("existing.txt", os.O_WRONLY|os.O_TRUNC, 0)
		if _, err := f.Write([]byte("new")); err != nil {
			t.Fatalf("Write: %v", err)
		}
		_ = f.Close()

		r := c.Open("existing.txt")
		defer func() { _ = r.Close() }()
		got, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if string(got) != "new" {
			t.Errorf("content after O_TRUNC: got %q, want %q", got, "new")
		}
	})

	t.Run("O_WRONLY|O_APPEND", func(t *testing.T) {
		c.SetupLines(`append.txt: "ab"`)
		f := c.OpenFile("append.txt", os.O_WRONLY|os.O_APPEND, 0)
		if _, err := f.Write([]byte("cd")); err != nil {
			t.Fatalf("Write: %v", err)
		}
		_ = f.Close()

		r := c.Open("append.txt")
		defer func() { _ = r.Close() }()
		got, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if string(got) != "abcd" {
			t.Errorf("content after O_APPEND: got %q, want %q", got, "abcd")
		}
	})

	t.Run("O_CREATE on new path", func(t *testing.T) {
		f := c.OpenFile("created.txt", os.O_WRONLY|os.O_CREATE, 0o644)
		_ = f.Close()
		if _, err := fsys.Stat("created.txt"); err != nil {
			t.Errorf("Stat after O_CREATE: %v", err)
		}
	})

	t.Run("O_CREATE|O_EXCL fails when path exists", func(t *testing.T) {
		c.SetupLines(`excl.txt: "x"`)
		f, err := fsys.OpenFile("excl.txt", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			_ = f.Close()
			t.Fatalf("O_CREATE|O_EXCL on existing path: want error, got nil")
		}
		if !errors.Is(err, fs.ErrExist) {
			t.Errorf("O_CREATE|O_EXCL: want fs.ErrExist, got %v", err)
		}
	})

	t.Run("non-existent path without O_CREATE", func(t *testing.T) {
		f, err := fsys.OpenFile("missing.txt", os.O_RDONLY, 0)
		if err == nil {
			_ = f.Close()
			t.Fatalf("OpenFile missing: want error, got nil")
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("OpenFile missing: want fs.ErrNotExist, got %v", err)
		}
	})

	t.Run("parent does not exist", func(t *testing.T) {
		f, err := fsys.OpenFile("missing-parent/file.txt", os.O_WRONLY|os.O_CREATE, 0o644)
		if err == nil {
			_ = f.Close()
			t.Fatalf("OpenFile missing parent: want error, got nil")
		}
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("OpenFile missing parent: want fs.ErrNotExist, got %v", err)
		}
	})
}
