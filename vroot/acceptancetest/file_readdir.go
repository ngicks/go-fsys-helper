package acceptancetest

import (
	"io"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileReadDir exercises [vroot.File.ReadDir].
//
// ReadDir returns directory entries as [fs.DirEntry]. With n<=0 it returns all entries
// at once. With n>0 it returns up to n entries per call, then io.EOF (or just nil with
// less-than-n on the final call, depending on implementation).
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

	t.Run("read all", func(t *testing.T) {
		f := c.Open("dir")
		defer func() { _ = f.Close() }()

		entries, err := f.ReadDir(-1)
		testhelper.NilErr(t, err)
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		slices.Sort(names)
		want := []string{"a.txt", "b.txt", "c.txt", "sub"}
		if !slices.Equal(names, want) {
			t.Errorf("names: got %v, want %v", names, want)
		}
	})

	t.Run("read in chunks", func(t *testing.T) {
		f := c.Open("dir")
		defer func() { _ = f.Close() }()

		var collected []string
		for {
			entries, err := f.ReadDir(2)
			for _, e := range entries {
				collected = append(collected, e.Name())
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("ReadDir(2): %v", err)
			}
			if len(entries) == 0 {
				// Some implementations return nil error with zero entries on EOF.
				break
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
