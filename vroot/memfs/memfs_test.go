package memfs_test

import (
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
	"github.com/ngicks/go-fsys-helper/vroot/synthfs"
)

// makeRoot constructs a fresh memfs Root and pre-populates it from lines via
// the standard testhelper SetupLines machinery. memfs.New is the only memfs
// API; the rest is delegated to synthfs.
func makeRoot(t *testing.T, lines []string) *synthfs.Root {
	t.Helper()
	r := memfs.New("memfs://")
	testhelper.New(t, r).SetupLines(lines...)
	return r
}

func TestRoot(t *testing.T) {
	s := acceptancetest.SetupRoot[vroot.File, *synthfs.Root]{
		Make: makeRoot,
		Option: acceptancetest.Option{
			ChownUid: 1000,
			ChownGid: 1000,
		},
	}
	acceptancetest.RunRoot(t, s)
}
