package acceptancetest

import (
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestClose exercises [vroot.Fs.Close].
//
// Implementations may make Close a no-op; this test just asserts the call returns nil
// on the first invocation. Behavior after Close (whether further operations succeed or
// fail) is implementation-defined and intentionally not asserted here.
func TestClose[F vroot.File, Fs vroot.Fs[F]](t *testing.T, s Setup[F, Fs]) {
	fsys := s.Make(t, nil)
	testhelper.NilErr(t, fsys.Close())
}
