package acceptancetest

import (
	"os"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestFileSync exercises [vroot.File.Sync].
//
// Sync flushes the file to stable storage. Implementations that don't have stable storage
// (e.g. in-memory) typically return nil.
func TestFileSync[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := makeFs(t, s)
	c := newC(t, fsys)

	c.SetupLines(`f.txt: "x"`)

	f := c.OpenFile("f.txt", os.O_WRONLY, 0)
	defer func() { _ = f.Close() }()

	_, err := f.Write([]byte("y"))
	testhelper.NilErr(t, err)
	testhelper.NilErr(t, f.Sync())
}
