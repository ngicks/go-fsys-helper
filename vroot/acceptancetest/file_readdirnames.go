package acceptancetest

import (
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileReaddirnames exercises [vroot.File.Readdirnames].
func TestFileReaddirnames[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(
		"dir/",
		`dir/a.txt: "a"`,
		`dir/b.txt: "b"`,
	)

	f := c.Open("dir")
	defer func() { _ = f.Close() }()

	names, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatalf("Readdirnames: %v", err)
	}
	slices.Sort(names)
	want := []string{"a.txt", "b.txt"}
	if !slices.Equal(names, want) {
		t.Errorf("names: got %v, want %v", names, want)
	}
}
