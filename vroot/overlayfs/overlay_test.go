package overlayfs_test

import (
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
	"github.com/ngicks/go-fsys-helper/vroot/overlayfs"
)

// overlayOption mirrors synthfs's acceptance option: memfs (the overlay's top
// and lowers) is pure-Go in-memory, so symlinks run on every platform and Chown
// stores uid/gid without real ownership backing.
func overlayOption() acceptancetest.Option {
	return acceptancetest.Option{
		ChownUid: 1000,
		ChownGid: 1000,
	}
}

// makeOverlay builds a fresh overlay (memfs top + two empty memfs lowers,
// ephemeral metadata, in-place DotTmp copy-up so no standing work dir trips
// strict listing assertions — DECISION D3/D4) and materializes lines THROUGH
// the overlay's own write methods.
func makeOverlay(t *testing.T, lines []string) *overlayfs.Fs {
	t.Helper()
	top := memfs.New("overlay://")
	lower1 := memfs.New("lower1")
	lower2 := memfs.New("lower2")
	o := overlayfs.New(
		overlayfs.NewLayer[vroot.File](top, overlayfs.NewMetadataStoreMem()),
		[]overlayfs.Layer{
			overlayfs.NewLayer[vroot.File](lower1, nil),
			overlayfs.NewLayer[vroot.File](lower2, nil),
		},
		&overlayfs.Option{CopyUpPolicy: overlayfs.NewCopyUpPolicyDotTmp(0)},
	)
	testhelper.New(t, o).SetupLines(lines...)
	return o
}

// TestRoot runs the full vroot.Root acceptance suite (read + write + escapes +
// race) against the overlay.
func TestRoot(t *testing.T) {
	s := acceptancetest.SetupRoot[vroot.File, *overlayfs.Fs]{
		Make:   makeOverlay,
		Option: overlayOption(),
	}
	acceptancetest.RunRoot(t, s)
}

// TestFs exercises the *overlayfs.Fs surface in addition to the Root surface.
func TestFs(t *testing.T) {
	s := acceptancetest.Setup[vroot.File, *overlayfs.Fs]{
		Make:   makeOverlay,
		Option: overlayOption(),
	}
	acceptancetest.RunFs(t, s)
}
