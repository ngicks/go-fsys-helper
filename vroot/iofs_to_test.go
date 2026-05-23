package vroot_test

import (
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

// setupIoFsToFixture creates a small file tree under t.TempDir and returns the
// osfs.Root scoped to it. The tree has no symlinks so fstest.TestFS's strict
// walk doesn't trip on out-of-tree targets.
//
// osfs.Root (via *os.Root) accepts forward-slash paths on every platform, so
// the adapter is wrapped directly with no PathLocalizer; the fs.ValidPath
// guard inside the adapter is what rejects fstest's "//", "." and ".." path
// mutants.
func setupIoFsToFixture(t *testing.T) *osfs.Root {
	t.Helper()
	dir := t.TempDir()
	setupFs, err := osfs.NewFs(dir)
	if err != nil {
		t.Fatalf("NewFs setup: %v", err)
	}
	testhelper.New[*testing.T, *os.File](t, setupFs).SetupLines(
		`file1.txt: "f1"`,
		`file2.txt: "f2"`,
		`subdir/`,
		`subdir/nested.txt: "n"`,
		`subdir/double_nested/`,
		`subdir/double_nested/deep.txt: "d"`,
	)
	r, err := osfs.NewRoot(dir)
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// TestToIoFsRoot exposes an osfs.Root as fs.FS and validates the surface with
// fstest.TestFS, which also asserts that non-fs.ValidPath inputs are rejected.
func TestToIoFsRoot(t *testing.T) {
	r := setupIoFsToFixture(t)
	iofs := vroot.ToIoFsRoot(r)

	if err := fstest.TestFS(iofs,
		"file1.txt",
		"file2.txt",
		"subdir/nested.txt",
		"subdir/double_nested/deep.txt",
	); err != nil {
		t.Fatal(err)
	}
}

// TestToIoFsRoot_Sub verifies fs.SubFS support: Sub yields a sub-filesystem
// rooted at the requested directory.
func TestToIoFsRoot_Sub(t *testing.T) {
	r := setupIoFsToFixture(t)
	iofs := vroot.ToIoFsRoot(r)

	subFS, ok := iofs.(fs.SubFS)
	if !ok {
		t.Fatal("ToIoFsRoot result does not implement fs.SubFS")
	}
	sub, err := subFS.Sub("subdir")
	if err != nil {
		t.Fatalf("Sub: %v", err)
	}
	if err := fstest.TestFS(sub, "nested.txt", "double_nested/deep.txt"); err != nil {
		t.Fatal(err)
	}
}

// TestToIoFs verifies the Fs-only adapter: implements fs.FS and the read
// extension interfaces but not fs.SubFS.
func TestToIoFs(t *testing.T) {
	dir := t.TempDir()
	setupFs, err := osfs.NewFs(dir)
	if err != nil {
		t.Fatalf("NewFs setup: %v", err)
	}
	testhelper.New[*testing.T, *os.File](t, setupFs).SetupLines(
		`file1.txt: "f1"`,
		`subdir/`,
		`subdir/nested.txt: "n"`,
	)
	fsys, err := osfs.NewFs(dir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	iofs := vroot.ToIoFs[*os.File](fsys)

	if err := fstest.TestFS(iofs, "file1.txt", "subdir/nested.txt"); err != nil {
		t.Fatal(err)
	}

	if _, ok := iofs.(fs.SubFS); ok {
		t.Error("ToIoFs result should not implement fs.SubFS (Fs has no scoping op)")
	}
}

// TestToIoFsRoot_ExtensionInterfaces type-asserts the optional fs.FS extension
// interfaces ToIoFsRoot is documented to expose.
func TestToIoFsRoot_ExtensionInterfaces(t *testing.T) {
	r := setupIoFsToFixture(t)
	iofs := vroot.ToIoFsRoot(r)

	if _, ok := iofs.(fs.ReadDirFS); !ok {
		t.Error("want fs.ReadDirFS")
	}
	if _, ok := iofs.(fs.ReadFileFS); !ok {
		t.Error("want fs.ReadFileFS")
	}
	if _, ok := iofs.(fs.ReadLinkFS); !ok {
		t.Error("want fs.ReadLinkFS")
	}
	if _, ok := iofs.(fs.StatFS); !ok {
		t.Error("want fs.StatFS")
	}
}
