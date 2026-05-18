package acceptancetest

import (
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileReaddir exercises [vroot.File.Readdir] (the FileInfo-returning variant).
func TestFileReaddir[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s,
		"dir/",
		`dir/a.txt: "a"`,
		`dir/b.txt: "b"`,
	)
	c := newC(t, fsys)

	f := c.Open("dir")
	defer func() { _ = f.Close() }()

	infos, err := f.Readdir(-1)
	testhelper.NilErr(t, err)
	names := make([]string, 0, len(infos))
	for _, i := range infos {
		names = append(names, i.Name())
	}
	slices.Sort(names)
	want := []string{"a.txt", "b.txt"}
	if !slices.Equal(names, want) {
		t.Errorf("names: got %v, want %v", names, want)
	}
}
