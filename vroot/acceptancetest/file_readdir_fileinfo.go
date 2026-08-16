package acceptancetest

import (
	"io"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileReaddir exercises [vroot.File.Readdir] (the FileInfo-returning
// variant). Like [TestFileReadDir], it verifies the cursor advances across
// multiple Readdir(n) calls (n>0) and treats mid-iteration directory
// mutations as implementation-defined.
func TestFileReaddir[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s,
		"dir/",
		`dir/a.txt: "a"`,
		`dir/b.txt: "b"`,
		`dir/c.txt: "c"`,
	)
	c := newC(t, fsys)

	f := c.Open("dir")
	defer func() { _ = f.Close() }()

	var names []string
	for {
		infos, err := f.Readdir(2)
		for _, i := range infos {
			names = append(names, i.Name())
		}
		if err == io.EOF || len(infos) == 0 {
			break
		}
		if err != nil {
			t.Fatalf("Readdir(2): %v", err)
		}
	}
	slices.Sort(names)
	want := []string{"a.txt", "b.txt", "c.txt"}
	if !slices.Equal(names, want) {
		t.Errorf("names: got %v, want %v", names, want)
	}
}
