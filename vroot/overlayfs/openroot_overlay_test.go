package overlayfs_test

import (
	"errors"
	"io/fs"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// TestOpenRootMaskingShared verifies that a sub-overlay derived via OpenRoot
// shares the parent's ovl-node table, so a whiteout set in the parent is visible
// through the sub-overlay, and a file created in the sub-overlay is visible in
// the parent at its full path. Confinement: the sub-overlay rejects escapes.
func TestOpenRootMaskingShared(t *testing.T) {
	o, _, _, _ := buildOverlay(t, nil, []string{
		"sub/",
		`sub/a.txt: "lower-a"`,
		`sub/b.txt: "lower-b"`,
	})
	defer func() { _ = o.Close() }()

	// Parent whites out sub/a.txt.
	if err := o.Remove("sub/a.txt"); err != nil {
		t.Fatalf("Remove sub/a.txt: %v", err)
	}

	o2, err := o.OpenRoot("sub")
	if err != nil {
		t.Fatalf("OpenRoot sub: %v", err)
	}

	// The whiteout (set in the parent) is visible through the sub-overlay: a.txt
	// is ENOENT, while the sibling b.txt remains readable.
	if _, err := o2.Stat("a.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("sub Stat a.txt: err = %v, want ErrNotExist (shared whiteout)", err)
	}
	if got := readAll(t, o2, "b.txt"); got != "lower-b" {
		t.Fatalf("sub b.txt = %q, want lower-b", got)
	}

	// A file created in the sub-overlay is visible in the parent at sub/c.txt.
	cf, err := o2.Create("c.txt")
	if err != nil {
		t.Fatalf("sub Create c.txt: %v", err)
	}
	if _, err := cf.WriteString("c-data"); err != nil {
		t.Fatalf("sub write c.txt: %v", err)
	}
	if err := cf.Close(); err != nil {
		t.Fatalf("sub close c.txt: %v", err)
	}
	if got := readAll(t, o, "sub/c.txt"); got != "c-data" {
		t.Fatalf("parent sub/c.txt = %q, want c-data", got)
	}

	// Confinement: the sub-overlay must not see or escape above its base.
	if _, err := o2.Stat("../sub/b.txt"); !errors.Is(err, vroot.ErrPathEscapes) {
		t.Fatalf("sub Stat ../sub/b.txt: err = %v, want ErrPathEscapes", err)
	}
	if _, err := o2.OpenRoot(".."); !errors.Is(err, vroot.ErrPathEscapes) {
		t.Fatalf("sub OpenRoot ..: err = %v, want ErrPathEscapes", err)
	}
}

// TestOpenRootListingMerged confirms a sub-overlay lists the merged view scoped
// to its base, omitting the parent-set whiteout.
func TestOpenRootListingMerged(t *testing.T) {
	o, _, _, _ := buildOverlay(t, nil, []string{
		"sub/",
		`sub/a.txt: "lower-a"`,
		`sub/b.txt: "lower-b"`,
	})
	defer func() { _ = o.Close() }()

	if err := o.Remove("sub/a.txt"); err != nil {
		t.Fatalf("Remove sub/a.txt: %v", err)
	}
	o2, err := o.OpenRoot("sub")
	if err != nil {
		t.Fatalf("OpenRoot sub: %v", err)
	}
	names := listNames(t, o2, ".")
	if slices.Contains(names, "a.txt") {
		t.Fatalf("sub listing %v must omit whited-out a.txt", names)
	}
	if !slices.Contains(names, "b.txt") {
		t.Fatalf("sub listing %v must contain b.txt", names)
	}
}
