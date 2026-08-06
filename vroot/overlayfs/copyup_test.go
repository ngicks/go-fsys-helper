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
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
	"github.com/ngicks/go-fsys-helper/vroot/overlayfs"
)

// buildOverlay constructs an overlay with the given top and lower fixture lines
// (testhelper syntax). The lower is populated directly (it is read-only through
// the overlay); the returned top memfs is the live top so tests can assert what
// was materialized there. Copy-up uses DotTmp (no standing work dir).
func buildOverlay(
	t *testing.T,
	topLines, lowerLines []string,
) (o *overlayfs.Fs, top *overlayfs.Fs, topFsys, lowerFsys vroot.Fs[vroot.File]) {
	t.Helper()
	tf := memfs.New("top")
	lf := memfs.New("lower")
	if len(lowerLines) > 0 {
		testhelper.New(t, lf).SetupLines(lowerLines...)
	}
	if len(topLines) > 0 {
		testhelper.New(t, tf).SetupLines(topLines...)
	}
	o = overlayfs.New(
		overlayfs.NewLayer[vroot.File](tf, overlayfs.NewMetadataStoreMem()),
		[]overlayfs.Layer{overlayfs.NewLayer[vroot.File](lf, nil)},
		&overlayfs.Option{CopyUpPolicy: overlayfs.NewCopyUpPolicyDotTmp(0)},
	)
	return o, o, tf, lf
}

func listNames(t *testing.T, o *overlayfs.Fs, dir string) []string {
	t.Helper()
	f, err := o.Open(dir)
	if err != nil {
		t.Fatalf("Open(%q): %v", dir, err)
	}
	defer func() { _ = f.Close() }()
	names, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatalf("Readdirnames(%q): %v", dir, err)
	}
	slices.Sort(names)
	return names
}

func mustNotExist(t *testing.T, o *overlayfs.Fs, name string) {
	t.Helper()
	_, err := o.Stat(name)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Stat(%q): want ErrNotExist, got %v", name, err)
	}
}

func topHas(t *testing.T, top vroot.Fs[vroot.File], name string) bool {
	t.Helper()
	_, err := top.Lstat(name)
	return err == nil
}

// TestCopyUpOnWrite: opening a lower-only file for write materializes it in top
// while leaving the lower untouched; the new content lands in top.
func TestCopyUpOnWrite(t *testing.T) {
	o, _, top, lower := buildOverlay(t, nil, []string{`a.txt: "lower-content"`})

	if topHas(t, top, "a.txt") {
		t.Fatal("a.txt must not be in top before write")
	}
	f, err := o.OpenFile("a.txt", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("OpenFile write: %v", err)
	}
	if _, err := f.WriteString("more"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !topHas(t, top, "a.txt") {
		t.Fatal("a.txt must be copied up to top after write")
	}
	// lower must be untouched.
	lf, err := lower.Open("a.txt")
	if err != nil {
		t.Fatalf("lower Open: %v", err)
	}
	lb, _ := io.ReadAll(lf)
	_ = lf.Close()
	if string(lb) != "lower-content" {
		t.Fatalf("lower mutated: %q", lb)
	}
	// merged read shows the top copy (lower-content + "more" written at offset 0
	// overwrote the first 4 bytes).
	got := readAll(t, o, "a.txt")
	if got[:4] != "more" {
		t.Fatalf("top copy content = %q, want prefix %q", got, "more")
	}
}

// TestCopyUpOnChmod: Chmod of a lower-only file copies it up and chmods the top
// copy.
func TestCopyUpOnChmod(t *testing.T) {
	o, _, top, _ := buildOverlay(t, nil, []string{`a.txt: "x"`})
	if err := o.Chmod("a.txt", 0o600); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	if !topHas(t, top, "a.txt") {
		t.Fatal("a.txt must be copied up by Chmod")
	}
	info, err := o.Stat("a.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o, want 600", info.Mode().Perm())
	}
}

// TestParentCopyUp: writing into a lower-only directory copies its parents up to
// top non-opaque (lower siblings stay visible).
func TestParentCopyUp(t *testing.T) {
	o, _, top, _ := buildOverlay(t, nil, []string{
		"a/",
		"a/b/",
		`a/b/existing.txt: "e"`,
	})
	f, err := o.OpenFile("a/b/new.txt", os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("OpenFile create nested: %v", err)
	}
	_, _ = f.WriteString("n")
	_ = f.Close()

	if !topHas(t, top, "a") || !topHas(t, top, "a/b") {
		t.Fatal("parent dirs a, a/b must be materialized in top")
	}
	// lower sibling stays visible (parent not opaque).
	names := listNames(t, o, "a/b")
	if !slices.Contains(names, "existing.txt") || !slices.Contains(names, "new.txt") {
		t.Fatalf("a/b listing = %v, want both existing.txt and new.txt", names)
	}
}

// TestWhiteout: removing a lower-only file hides it (ENOENT), a sibling stays
// visible, and listings omit it.
func TestWhiteout(t *testing.T) {
	o, _, _, _ := buildOverlay(t, nil, []string{
		`a.txt: "a"`,
		`b.txt: "b"`,
	})
	if err := o.Remove("a.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	mustNotExist(t, o, "a.txt")
	if got := readAll(t, o, "b.txt"); got != "b" {
		t.Fatalf("b.txt = %q, want b", got)
	}
	names := listNames(t, o, ".")
	if slices.Contains(names, "a.txt") {
		t.Fatalf("root listing %v must not contain a.txt", names)
	}
	if !slices.Contains(names, "b.txt") {
		t.Fatalf("root listing %v must contain b.txt", names)
	}
}

// TestOpaqueOnRecreate is the regression test for the old delete-then-recreate
// bug (decision 13): RemoveAll a lower-backed dir then recreate it; lower
// children must stay hidden while new top children show.
func TestOpaqueOnRecreate(t *testing.T) {
	o, _, _, _ := buildOverlay(t, nil, []string{
		"d/",
		`d/x.txt: "x"`,
	})
	if err := o.RemoveAll("d"); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	mustNotExist(t, o, "d/x.txt")
	if err := o.Mkdir("d", 0o755); err != nil {
		t.Fatalf("Mkdir recreate: %v", err)
	}
	f, err := o.OpenFile("d/y.txt", os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatalf("create d/y.txt: %v", err)
	}
	_, _ = f.WriteString("y")
	_ = f.Close()

	// x.txt (lower) stays hidden; y.txt (top) shows.
	mustNotExist(t, o, "d/x.txt")
	names := listNames(t, o, "d")
	if slices.Contains(names, "x.txt") {
		t.Fatalf("d listing %v must NOT contain the lower's x.txt (opaque)", names)
	}
	if !slices.Contains(names, "y.txt") {
		t.Fatalf("d listing %v must contain y.txt", names)
	}
}

// TestRecreateOverWhiteoutClearsIt: removing a lower-only file then creating a
// new file at the same name clears the whiteout (the old bug's simplest form).
func TestRecreateOverWhiteoutClearsIt(t *testing.T) {
	o, _, _, _ := buildOverlay(t, nil, []string{`f.txt: "old"`})
	if err := o.Remove("f.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	mustNotExist(t, o, "f.txt")
	f, err := o.Create("f.txt")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, _ = f.WriteString("new")
	_ = f.Close()
	if got := readAll(t, o, "f.txt"); got != "new" {
		t.Fatalf("f.txt = %q, want new (whiteout must be cleared)", got)
	}
}

// TestCrossLayerRead: a file present only in a lower reads through the overlay;
// resolution prefers top when both have it.
func TestCrossLayerRead(t *testing.T) {
	o, _, _, _ := buildOverlay(t, []string{`shared.txt: "top"`}, []string{
		`lower-only.txt: "low"`,
		`shared.txt: "low"`,
	})
	if got := readAll(t, o, "lower-only.txt"); got != "low" {
		t.Fatalf("lower-only.txt = %q, want low", got)
	}
	if got := readAll(t, o, "shared.txt"); got != "top" {
		t.Fatalf("shared.txt = %q, want top (top wins)", got)
	}
}

// TestRenameCopiesUpAndWhitesSource: renaming a lower-only file materializes the
// destination in top and whites out the source name.
func TestRenameCopiesUpAndWhitesSource(t *testing.T) {
	o, _, top, _ := buildOverlay(t, nil, []string{`src.txt: "data"`})
	if err := o.Rename("src.txt", "dst.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if got := readAll(t, o, "dst.txt"); got != "data" {
		t.Fatalf("dst.txt = %q, want data", got)
	}
	mustNotExist(t, o, "src.txt")
	if !topHas(t, top, "dst.txt") {
		t.Fatal("dst.txt must exist in top")
	}
	names := listNames(t, o, ".")
	if slices.Contains(names, "src.txt") {
		t.Fatalf("root listing %v must not contain src.txt", names)
	}
}
