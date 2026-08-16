package acceptancetest

import (
	"io"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileReadDir exercises [vroot.File.ReadDir].
//
// ReadDir is stateful: successive ReadDir(n) calls (n>0) advance an internal
// cursor and return non-overlapping batches until exhausted (io.EOF or zero
// entries). Behavior when the underlying directory is mutated mid-iteration is
// implementation-defined and not exercised here.
func TestFileReadDir[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s,
		"dir/",
		`dir/a.txt: "a"`,
		`dir/b.txt: "b"`,
		`dir/c.txt: "c"`,
		"dir/sub/",
		`reg.txt: "x"`,
	)
	c := newC(t, fsys)

	t.Run("stateful chunked read", func(t *testing.T) {
		f := c.Open("dir")
		defer func() { _ = f.Close() }()

		var collected []string
		for {
			entries, err := f.ReadDir(2)
			for _, e := range entries {
				collected = append(collected, e.Name())
			}
			if err == io.EOF || len(entries) == 0 {
				break
			}
			if err != nil {
				t.Fatalf("ReadDir(2): %v", err)
			}
		}
		slices.Sort(collected)
		want := []string{"a.txt", "b.txt", "c.txt", "sub"}
		if !slices.Equal(collected, want) {
			t.Errorf("collected names: got %v, want %v", collected, want)
		}
	})

	t.Run("ReadDir on regular file errors", func(t *testing.T) {
		f := c.Open("reg.txt")
		defer func() { _ = f.Close() }()
		if _, err := f.ReadDir(-1); err == nil {
			t.Errorf("ReadDir on regular file: want error, got nil")
		}
	})
}
