package synthfs_test

import (
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/synthfs"
)

// synthfsOption builds an acceptance-suite option for synthfs.
//
// Symlinks are exercised on every platform (synthfs is pure-Go in-memory, so
// Windows symlink-privilege concerns don't apply). Chown is exercised but
// only sets stored uid/gid — there's no real ownership backing it.
func synthfsOption() acceptancetest.Option {
	return acceptancetest.Option{
		// Auto-detect Os from runtime.GOOS via the suite's resolver.
		ChownUid: 1000,
		ChownGid: 1000,
	}
}

// makeRoot constructs a fresh synthfs.Root and pre-populates it from lines
// using the existing testhelper SetupLines machinery. matches the contract
// of acceptancetest.SetupRoot.Make.
func makeRoot(t *testing.T, lines []string) *synthfs.Root {
	t.Helper()
	mask := fsutil.MaskChmodMode
	// On Windows we need the OS-emulating chmod mask so the suite's "readonly
	// toggle" test works. On Unix the simple-perm default is fine but using
	// fsutil.MaskChmodMode keeps behavior consistent across platforms.
	r := synthfs.NewRoot("synth://", &synthfs.Option{MaskChmodMode: mask})
	testhelper.New(t, r).SetupLines(lines...)
	return r
}

func TestRoot(t *testing.T) {
	s := acceptancetest.SetupRoot[vroot.File, *synthfs.Root]{
		Make:   makeRoot,
		Option: synthfsOption(),
	}
	acceptancetest.RunRoot(t, s)
}

func TestFs(t *testing.T) {
	// *synthfs.Root also satisfies vroot.Fs[vroot.File]; exercise the Fs
	// surface in addition to the Root surface so both code paths run.
	s := acceptancetest.Setup[vroot.File, *synthfs.Root]{
		Make:   makeRoot,
		Option: synthfsOption(),
	}
	acceptancetest.RunFs(t, s)
}
