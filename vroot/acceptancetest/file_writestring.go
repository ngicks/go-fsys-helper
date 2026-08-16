package acceptancetest

import (
	"io"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileWriteString exercises [vroot.File.WriteString].
func TestFileWriteString[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	f := c.Create("s.txt")
	want := "hello string"
	n, err := f.WriteString(want)
	testhelper.NilErr(t, err)
	if n != len(want) {
		t.Errorf("WriteString returned n=%d, want %d", n, len(want))
	}
	_ = f.Close()

	r := c.Open("s.txt")
	defer func() { _ = r.Close() }()
	got, err := io.ReadAll(r)
	testhelper.NilErr(t, err)
	if string(got) != want {
		t.Errorf("content: got %q, want %q", got, want)
	}
}
