package acceptancetest

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileReadAt exercises [vroot.File.ReadAt].
//
// When Option.SkipReadAt is set, the implementation may return [vroot.ErrOpNotSupported].
// Otherwise ReadAt must not change the file position observed by sequential Read.
func TestFileReadAt[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(`f.txt: "abcdef"`)

	f := c.Open("f.txt")
	defer func() { _ = f.Close() }()

	if s.Option.SkipReadAt {
		buf := make([]byte, 1)
		_, err := f.ReadAt(buf, 0)
		if err != nil && !errors.Is(err, vroot.ErrOpNotSupported) {
			t.Errorf("ReadAt on unsupported file: want ErrOpNotSupported or success, got %v", err)
		}
		return
	}

	t.Run("reads at offset", func(t *testing.T) {
		buf := make([]byte, 3)
		n, err := f.ReadAt(buf, 2)
		if err != nil && err != io.EOF {
			t.Fatalf("ReadAt: %v", err)
		}
		if n != 3 || !bytes.Equal(buf, []byte("cde")) {
			t.Errorf("ReadAt: n=%d buf=%q, want 3 %q", n, buf, "cde")
		}
	})

	t.Run("ReadAt does not affect sequential Read", func(t *testing.T) {
		// Re-open so the cursor starts at 0.
		f := c.Open("f.txt")
		defer func() { _ = f.Close() }()

		buf := make([]byte, 3)
		if _, err := f.ReadAt(buf, 3); err != nil && err != io.EOF {
			t.Fatalf("ReadAt: %v", err)
		}

		got, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if !bytes.Equal(got, []byte("abcdef")) {
			t.Errorf("after ReadAt, ReadAll: got %q, want %q", got, "abcdef")
		}
	})

	t.Run("EOF past end", func(t *testing.T) {
		buf := make([]byte, 4)
		n, err := f.ReadAt(buf, 4)
		if err != io.EOF {
			t.Errorf("ReadAt past end: want io.EOF, got %v", err)
		}
		if n != 2 {
			t.Errorf("ReadAt past end: n=%d, want 2", n)
		}
	})
}
