package acceptancetest

import (
	"io"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileReaddirnames exercises [vroot.File.Readdirnames]. Like
// [TestFileReadDir], it verifies the cursor advances across multiple
// Readdirnames(n) calls (n>0) and treats mid-iteration directory mutations as
// implementation-defined.
func TestFileReaddirnames[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s,
		"dir/",
		`dir/a.txt: "a"`,
		`dir/b.txt: "b"`,
		`dir/c.txt: "c"`,
	)
	c := newC(t, fsys)

	f := c.Open("dir")
	defer func() { _ = f.Close() }()

	var collected []string
	for {
		batch, err := f.Readdirnames(2)
		collected = append(collected, batch...)
		if err == io.EOF || len(batch) == 0 {
			break
		}
		if err != nil {
			t.Fatalf("Readdirnames(2): %v", err)
		}
	}
	slices.Sort(collected)
	want := []string{"a.txt", "b.txt", "c.txt"}
	if !slices.Equal(collected, want) {
		t.Errorf("names: got %v, want %v", collected, want)
	}
}
