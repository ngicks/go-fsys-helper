package overlayfs_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
	"github.com/ngicks/go-fsys-helper/vroot/overlayfs"
)

// makeReadOverlay builds a read-only-populated overlay: the TOP is populated
// out-of-band (the overlay's own write methods are stubbed in Wave 3a), then
// wrapped. No lowers for the baseline read suite.
func makeReadOverlay(t *testing.T, lines []string) *overlayfs.Fs {
	t.Helper()
	top := memfs.New("top")
	testhelper.New(t, top).SetupLines(lines...)
	return overlayfs.New(
		overlayfs.NewLayer[vroot.File](top, overlayfs.NewMetadataStoreMem()),
		nil,
		&overlayfs.Option{CopyUpPolicy: overlayfs.NewCopyUpPolicyDotTmp(0)},
	)
}

func TestReadOnlyAcceptance(t *testing.T) {
	acceptancetest.RunRootReadOnly(t, acceptancetest.SetupRoot[vroot.File, *overlayfs.Fs]{
		Make:   makeReadOverlay,
		Option: acceptancetest.Option{ChownUid: 1000, ChownGid: 1000},
	})
}

// readAll opens name through f and returns its full contents.
func readAll(t *testing.T, f *overlayfs.Fs, name string) string {
	t.Helper()
	file, err := f.Open(name)
	if err != nil {
		t.Fatalf("Open(%q): %v", name, err)
	}
	defer func() { _ = file.Close() }()
	b, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("ReadAll(%q): %v", name, err)
	}
	return string(b)
}

// dirNames returns the sorted entry names of the directory name read through f.
func dirNames(t *testing.T, f *overlayfs.Fs, name string) []string {
	t.Helper()
	d, err := f.Open(name)
	if err != nil {
		t.Fatalf("Open dir %q: %v", name, err)
	}
	defer func() { _ = d.Close() }()
	ents, err := d.ReadDir(-1)
	if err != nil {
		t.Fatalf("ReadDir %q: %v", name, err)
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		names = append(names, e.Name())
	}
	slices.Sort(names)
	return names
}

func TestMergedLowerRead(t *testing.T) {
	top := memfs.New("top")
	lower := memfs.New("lower")
	testhelper.New(t, lower).SetupLines(
		`a.txt: "from-lower"`,
		"d/",
		`d/x.txt: "x-from-lower"`,
	)
	ov := overlayfs.New(
		overlayfs.NewLayer[vroot.File](top, overlayfs.NewMetadataStoreMem()),
		[]overlayfs.Layer{overlayfs.NewLayer[vroot.File](lower, nil)},
		nil,
	)
	defer func() { _ = ov.Close() }()

	if got := readAll(t, ov, "a.txt"); got != "from-lower" {
		t.Errorf("read a.txt = %q, want %q", got, "from-lower")
	}
	if _, err := ov.Stat("a.txt"); err != nil {
		t.Errorf("Stat a.txt: %v", err)
	}
	// Merged listing of root includes the lower-only file and dir.
	if names := dirNames(t, ov, "."); !slices.Equal(names, []string{"a.txt", "d"}) {
		t.Errorf("root listing = %v, want [a.txt d]", names)
	}
	if got := readAll(t, ov, "d/x.txt"); got != "x-from-lower" {
		t.Errorf("read d/x.txt = %q, want %q", got, "x-from-lower")
	}
}

func TestResolutionOrderTopWins(t *testing.T) {
	top := memfs.New("top")
	lower := memfs.New("lower")
	testhelper.New(t, top).SetupLines(
		`shared.txt: "from-top"`,
	)
	testhelper.New(t, lower).SetupLines(
		`shared.txt: "from-lower"`,
	)
	ov := overlayfs.New(
		overlayfs.NewLayer[vroot.File](top, overlayfs.NewMetadataStoreMem()),
		[]overlayfs.Layer{overlayfs.NewLayer[vroot.File](lower, nil)},
		nil,
	)
	defer func() { _ = ov.Close() }()

	if got := readAll(t, ov, "shared.txt"); got != "from-top" {
		t.Errorf("top must win: read = %q, want %q", got, "from-top")
	}
}

func TestResolutionOrderHigherLowerWins(t *testing.T) {
	top := memfs.New("top")
	low0 := memfs.New("low0") // lowest priority
	low1 := memfs.New("low1") // highest priority (last)
	testhelper.New(t, low0).SetupLines(
		`shared.txt: "from-low0"`,
	)
	testhelper.New(t, low1).SetupLines(
		`shared.txt: "from-low1"`,
	)
	ov := overlayfs.New(
		overlayfs.NewLayer[vroot.File](top, overlayfs.NewMetadataStoreMem()),
		[]overlayfs.Layer{
			overlayfs.NewLayer[vroot.File](low0, nil),
			overlayfs.NewLayer[vroot.File](low1, nil),
		},
		nil,
	)
	defer func() { _ = ov.Close() }()

	if got := readAll(t, ov, "shared.txt"); got != "from-low1" {
		t.Errorf("highest-priority lower must win: read = %q, want %q", got, "from-low1")
	}
}

func TestWhiteoutRead(t *testing.T) {
	top := memfs.New("top")
	lower := memfs.New("lower")
	testhelper.New(t, lower).SetupLines(
		`a.txt: "a"`,
		`b.txt: "b"`,
	)
	// Preseed the store so the ovl-node table is seeded with a.txt whited out.
	store := overlayfs.NewMetadataStoreMem()
	if err := store.SetWhiteout("a.txt"); err != nil {
		t.Fatalf("SetWhiteout: %v", err)
	}
	ov := overlayfs.New(
		overlayfs.NewLayer[vroot.File](top, store),
		[]overlayfs.Layer{overlayfs.NewLayer[vroot.File](lower, nil)},
		nil,
	)
	defer func() { _ = ov.Close() }()

	if _, err := ov.Stat("a.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat whited a.txt: err = %v, want ErrNotExist", err)
	}
	// Sibling remains visible.
	if _, err := ov.Stat("b.txt"); err != nil {
		t.Errorf("Stat sibling b.txt: %v", err)
	}
	if names := dirNames(t, ov, "."); !slices.Equal(names, []string{"b.txt"}) {
		t.Errorf("listing omits whited entry: got %v, want [b.txt]", names)
	}
}

func TestOpaqueRead(t *testing.T) {
	top := memfs.New("top")
	lower := memfs.New("lower")
	// top has dir d with y.txt; lower has dir d with x.txt.
	testhelper.New(t, top).SetupLines(
		"d/",
		`d/y.txt: "y-top"`,
	)
	testhelper.New(t, lower).SetupLines(
		"d/",
		`d/x.txt: "x-lower"`,
	)
	store := overlayfs.NewMetadataStoreMem()
	if err := store.SetOpaque("d"); err != nil {
		t.Fatalf("SetOpaque: %v", err)
	}
	ov := overlayfs.New(
		overlayfs.NewLayer[vroot.File](top, store),
		[]overlayfs.Layer{overlayfs.NewLayer[vroot.File](lower, nil)},
		nil,
	)
	defer func() { _ = ov.Close() }()

	// Opaque d: top child y.txt shows; lower child x.txt is hidden.
	if names := dirNames(t, ov, "d"); !slices.Equal(names, []string{"y.txt"}) {
		t.Errorf("opaque dir listing = %v, want [y.txt]", names)
	}
	if _, err := ov.Stat("d/x.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat opaque-hidden d/x.txt: err = %v, want ErrNotExist", err)
	}
	if _, err := ov.Stat("d/y.txt"); err != nil {
		t.Errorf("Stat top d/y.txt: %v", err)
	}
}

func TestNodeOnOpenAndGC(t *testing.T) {
	top := memfs.New("top")
	testhelper.New(t, top).SetupLines(
		`f.txt: "hello"`,
	)
	ov := overlayfs.New(
		overlayfs.NewLayer[vroot.File](top, overlayfs.NewMetadataStoreMem()),
		nil,
		nil,
	)
	defer func() { _ = ov.Close() }()

	// A bare Stat does NOT anchor a node.
	if _, err := ov.Stat("f.txt"); err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := overlayfs.HandleCountForTest(ov, "f.txt"); got != 0 {
		t.Errorf("bare Stat anchored a node: handleCount = %d, want 0", got)
	}

	// Open anchors a node and bumps the handle count.
	file, err := ov.OpenFile("f.txt", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got := overlayfs.HandleCountForTest(ov, "f.txt"); got != 1 {
		t.Errorf("after Open: handleCount = %d, want 1", got)
	}

	// Close drops the count to 0 and GCs the (unmasked) node.
	if err := file.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := overlayfs.HandleCountForTest(ov, "f.txt"); got != 0 {
		t.Errorf("after Close: handleCount = %d, want 0", got)
	}
	if overlayfs.HasNodeForTest(ov, "f.txt") {
		t.Errorf("after last Close: unmasked node should be GC'd")
	}
}
