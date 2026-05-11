package acceptancetest

import (
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileChown exercises [vroot.File.Chown].
func TestFileChown[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	if s.Option.SkipChown {
		t.Skip("SkipChown is set")
	}

	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(`f.txt: "x"`)

	f := c.Open("f.txt")
	defer func() { _ = f.Close() }()

	if err := f.Chown(s.Option.ChownUid, s.Option.ChownGid); err != nil {
		t.Fatalf("File.Chown: %v", err)
	}
}
