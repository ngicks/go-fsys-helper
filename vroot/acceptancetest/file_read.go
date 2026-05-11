package acceptancetest

import (
	"bytes"
	"io"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileRead exercises [vroot.File.Read].
//
// Reads return bytes sequentially. After exhausting the file, the next Read returns
// (0, io.EOF). Read on a closed file errors.
func TestFileRead[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	content := []byte("abcdef")
	c.SetupLines(`f.txt: "abcdef"`)

	t.Run("reads full content", func(t *testing.T) {
		f := c.Open("f.txt")
		defer func() { _ = f.Close() }()

		got, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if !bytes.Equal(got, content) {
			t.Errorf("content: got %q, want %q", got, content)
		}
	})

	t.Run("reads in chunks then EOF", func(t *testing.T) {
		f := c.Open("f.txt")
		defer func() { _ = f.Close() }()

		buf := make([]byte, 4)
		n, err := f.Read(buf)
		if err != nil {
			t.Fatalf("first Read: %v", err)
		}
		if n != 4 || !bytes.Equal(buf[:n], content[:4]) {
			t.Errorf("first Read: n=%d buf=%q, want 4 %q", n, buf[:n], content[:4])
		}

		n, err = f.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatalf("second Read: %v", err)
		}
		if n != 2 || !bytes.Equal(buf[:n], content[4:]) {
			t.Errorf("second Read: n=%d buf=%q, want 2 %q", n, buf[:n], content[4:])
		}

		// Drain final EOF (some implementations return io.EOF along with n>0).
		_, err = f.Read(buf)
		if err != io.EOF {
			t.Errorf("trailing Read: want io.EOF, got %v", err)
		}
	})
}
