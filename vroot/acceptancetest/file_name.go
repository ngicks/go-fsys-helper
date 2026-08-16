package acceptancetest

import (
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileName exercises [vroot.File.Name].
func TestFileName[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s, `hello.txt: "x"`)
	c := newC(t, fsys)

	f := c.Open("hello.txt")
	defer func() { _ = f.Close() }()

	if name := f.Name(); name == "" {
		t.Errorf("File.Name returned empty string")
	}
}
