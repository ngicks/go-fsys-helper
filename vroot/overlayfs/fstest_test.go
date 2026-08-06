package overlayfs_test

import (
	"os"
	"testing"
	"testing/fstest"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
	"github.com/ngicks/go-fsys-helper/vroot/overlayfs"
)

// TestFstestMergedView runs the stdlib fstest.TestFS conformance suite over the
// overlay's merged view. The tree mixes top-only, lower-only, and a masked
// (whited-out) lower file. DotTmp policy is used so no standing work dir appears
// in the merged listing (which would otherwise break TestFS's expected set).
func TestFstestMergedView(t *testing.T) {
	top := memfs.New("top")
	lower := memfs.New("lower")

	// Lower holds two files plus a dir; masked.txt will be whited out so it must
	// NOT appear in the merged view.
	testhelper.New(t, lower).SetupLines(
		`lower-only.txt: "lo"`,
		`masked.txt: "should-be-hidden"`,
		"d/",
		`d/deep.txt: "deep-lower"`,
	)

	o := overlayfs.New(
		overlayfs.NewLayer[vroot.File](top, overlayfs.NewMetadataStoreMem()),
		[]overlayfs.Layer{overlayfs.NewLayer[vroot.File](lower, nil)},
		&overlayfs.Option{CopyUpPolicy: overlayfs.NewCopyUpPolicyDotTmp(0)},
	)
	defer func() { _ = o.Close() }()

	// Top-only content written THROUGH the overlay.
	f, err := o.OpenFile("top-only.txt", os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("create top-only.txt: %v", err)
	}
	if _, err := f.WriteString("to"); err != nil {
		t.Fatalf("write top-only.txt: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close top-only.txt: %v", err)
	}

	// Whiteout: remove a lower file so it is masked in the merged view.
	if err := o.Remove("masked.txt"); err != nil {
		t.Fatalf("Remove masked.txt: %v", err)
	}

	// The full set of files fstest.TestFS should find. masked.txt is deliberately
	// absent (it is whited out and must not be visible).
	expected := []string{
		"top-only.txt",
		"lower-only.txt",
		"d/deep.txt",
	}
	if err := fstest.TestFS(vroot.ToIoFs[vroot.File](o), expected...); err != nil {
		t.Fatalf("fstest.TestFS: %v", err)
	}

	// Guard against the masked file leaking back in via the expected set itself.
	mustNotExist(t, o, "masked.txt")
}
