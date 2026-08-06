package overlayfs_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
	"github.com/ngicks/go-fsys-helper/vroot/overlayfs"
)

// makeStageOverlay builds an overlay using the default (visible) Stage work-dir
// policy, with a single memfs lower seeded by lowerLines. The live top memfs is
// returned so tests can inspect the work dir directly. The returned *Fs is the
// live overlay; the caller owns Close.
func makeStageOverlay(
	t *testing.T,
	lowerLines []string,
) (o *overlayfs.Fs, top vroot.Fs[vroot.File]) {
	t.Helper()
	tf := memfs.New("top")
	lf := memfs.New("lower")
	if len(lowerLines) > 0 {
		testhelper.New(t, lf).SetupLines(lowerLines...)
	}
	o = overlayfs.New(
		overlayfs.NewLayer[vroot.File](tf, overlayfs.NewMetadataStoreMem()),
		[]overlayfs.Layer{overlayfs.NewLayer[vroot.File](lf, nil)},
		&overlayfs.Option{CopyUpPolicy: overlayfs.NewCopyUpPolicyStage("")},
	)
	return o, tf
}

// TestStageWorkDirVisibleAndClean verifies the Stage policy's contract
// (PLAN decision 26 / §5.6): a copy-up succeeds, the default work dir is VISIBLE
// in the overlay's root listing (not hidden), and no leftover *.copyup.tmp
// remains in the work dir after the copy-up completes.
func TestStageWorkDirVisibleAndClean(t *testing.T) {
	o, top := makeStageOverlay(t, []string{`a.txt: "lower-a"`})
	defer func() { _ = o.Close() }()

	// Trigger a copy-up of the lower-only file via Chmod.
	if err := o.Chmod("a.txt", 0o600); err != nil {
		t.Fatalf("Chmod a.txt: %v", err)
	}

	// (a) Copy-up succeeded: a.txt is in top and readable through the overlay.
	if !topHas(t, top, "a.txt") {
		t.Fatal("a.txt must be copied up into top")
	}
	if got := readAll(t, o, "a.txt"); got != "lower-a" {
		t.Fatalf("a.txt = %q, want lower-a", got)
	}

	// (b) The work dir is VISIBLE in the overlay's root listing.
	names := listNames(t, o, ".")
	if !slices.Contains(names, overlayfs.DefaultWorkDir) {
		t.Fatalf("root listing %v must contain the visible work dir %q",
			names, overlayfs.DefaultWorkDir)
	}

	// (c) No leftover *.copyup.tmp in the work dir after the copy-up completes.
	leftovers := workDirCopyupTemps(t, o)
	if len(leftovers) != 0 {
		t.Fatalf("work dir has leftover copy-up temps: %v", leftovers)
	}
}

// workDirCopyupTemps lists entries in the overlay's default work dir whose names
// end in the copy-up temp suffix.
func workDirCopyupTemps(t *testing.T, o *overlayfs.Fs) []string {
	t.Helper()
	names := listNames(t, o, overlayfs.DefaultWorkDir)
	var temps []string
	for _, n := range names {
		if strings.HasSuffix(n, ".copyup.tmp") {
			temps = append(temps, n)
		}
	}
	return temps
}

// TestStageWorkDirSharedAcrossSubOverlay verifies that a copy-up done THROUGH a
// sub-overlay (OpenRoot) lands its scratch in the SAME single work dir at the
// top root, not in a per-sub-overlay dir (PLAN §5.6 "one work dir, shared").
func TestStageWorkDirSharedAcrossSubOverlay(t *testing.T) {
	o, top := makeStageOverlay(t, []string{
		"sub/",
		`sub/b.txt: "lower-b"`,
	})
	defer func() { _ = o.Close() }()

	o2, err := o.OpenRoot("sub")
	if err != nil {
		t.Fatalf("OpenRoot sub: %v", err)
	}

	// Copy-up b.txt through the sub-overlay.
	if err := o2.Chmod("b.txt", 0o600); err != nil {
		t.Fatalf("sub Chmod b.txt: %v", err)
	}
	if !topHas(t, top, "sub/b.txt") {
		t.Fatal("sub/b.txt must be copied up into top")
	}

	// The single work dir is at the TOP root (not sub/.vroot-overlayfs.work):
	// it is visible in the parent overlay's root listing, and there is no
	// sub-scoped work dir under sub/.
	parentNames := listNames(t, o, ".")
	if !slices.Contains(parentNames, overlayfs.DefaultWorkDir) {
		t.Fatalf("parent root listing %v must contain the shared work dir %q",
			parentNames, overlayfs.DefaultWorkDir)
	}
	subNames := listNames(t, o, "sub")
	if slices.Contains(subNames, overlayfs.DefaultWorkDir) {
		t.Fatalf("sub dir listing %v must NOT contain its own work dir "+
			"(the one work dir is shared at the top root)", subNames)
	}
}
