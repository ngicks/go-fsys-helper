package acceptancetest

import (
	"io"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileWrite exercises [vroot.File.Write].
func TestFileWrite[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	t.Run("Create+Write+Reopen+Read", func(t *testing.T) {
		f := c.Create("w.txt")
		want := []byte("hello world")
		n, err := f.Write(want)
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
		if n != len(want) {
			t.Errorf("Write returned n=%d, want %d", n, len(want))
		}
		_ = f.Close()

		r := c.Open("w.txt")
		defer func() { _ = r.Close() }()
		got, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if string(got) != string(want) {
			t.Errorf("content: got %q, want %q", got, want)
		}
	})

	t.Run("multiple Writes append in sequence", func(t *testing.T) {
		f := c.Create("multi.txt")
		for _, chunk := range []string{"foo", "bar", "baz"} {
			if _, err := f.Write([]byte(chunk)); err != nil {
				t.Fatalf("Write: %v", err)
			}
		}
		_ = f.Close()

		r := c.Open("multi.txt")
		defer func() { _ = r.Close() }()
		got, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if string(got) != "foobarbaz" {
			t.Errorf("content: got %q, want %q", got, "foobarbaz")
		}
	})
}
