package vroot_test

import (
	"io/fs"
	"os"
	"runtime"
	"testing"
	"testing/fstest"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func iofsFromOption() acceptancetest.Option {
	return acceptancetest.Option{
		// FromIoFs/FromIoFsRoot are read-only; the read-only suite never
		// exercises chown/chmod, but symlink reads are exercised.
		SkipSymlink: runtime.GOOS == "windows" && os.Getenv("GITHUB_ACTIONS") != "true",
		SkipChown:   runtime.GOOS == "windows",
		ChownUid:    os.Getuid(),
		ChownGid:    os.Getgid(),
	}
}

// materializeDirFS creates a fresh temp dir, applies lines to it, and returns
// an fs.ReadLinkFS view via os.DirFS (which implements fs.ReadLinkFS as of
// Go 1.25).
func materializeDirFS(t *testing.T, lines []string) fs.ReadLinkFS {
	t.Helper()
	dir := t.TempDir()
	setupFs, err := osfs.NewFs(dir)
	if err != nil {
		t.Fatalf("NewFs setup: %v", err)
	}
	testhelper.New[*testing.T, *os.File](t, setupFs).SetupLines(lines...)
	dirFS, ok := os.DirFS(dir).(fs.ReadLinkFS)
	if !ok {
		t.Skip("os.DirFS does not implement fs.ReadLinkFS on this Go version")
	}
	return dirFS
}

// TestFromIoFs runs the read-only Fs acceptance suite against an fs.FS adapted
// via FromIoFs. The wrapper's concrete type is unexported, so the suite is
// instantiated through the Fs[vroot.File] interface.
func TestFromIoFs(t *testing.T) {
	s := acceptancetest.Setup[vroot.File, vroot.Fs[vroot.File]]{
		Make: func(t *testing.T, lines []string) vroot.Fs[vroot.File] {
			return vroot.FromIoFs(materializeDirFS(t, lines), "fs.FS")
		},
		Option: iofsFromOption(),
	}
	acceptancetest.RunFsReadOnly(t, s)
}

// TestFromIoFsRoot runs the read-only Fs acceptance suite against the Root
// adapter (which also satisfies Fs[vroot.File]); RunRootReadOnly itself can't
// be used because its R type parameter would have to name the unexported
// concrete type. OpenRoot is covered separately by TestFromIoFsRoot_OpenRoot.
func TestFromIoFsRoot(t *testing.T) {
	s := acceptancetest.Setup[vroot.File, vroot.Fs[vroot.File]]{
		Make: func(t *testing.T, lines []string) vroot.Fs[vroot.File] {
			return vroot.FromIoFsRoot(materializeDirFS(t, lines), "fs.FS")
		},
		Option: iofsFromOption(),
	}
	acceptancetest.RunFsReadOnly(t, s)
}

// TestFromIoFsRoot_OpenRoot covers the Root-specific OpenRoot path: a sub-root
// is scoped to the requested directory and remains read-only.
func TestFromIoFsRoot_OpenRoot(t *testing.T) {
	dirFS := materializeDirFS(t, []string{
		"sub/",
		`sub/inner.txt: "inner"`,
		`top.txt: "top"`,
	})
	r := vroot.FromIoFsRoot(dirFS, "fs.FS")

	sub, err := r.OpenRoot("sub")
	if err != nil {
		t.Fatalf("OpenRoot: %v", err)
	}
	defer func() { _ = sub.Close() }()

	data, err := vroot.ReadFile(sub, "inner.txt")
	if err != nil {
		t.Fatalf("ReadFile(inner.txt) in sub-root: %v", err)
	}
	if string(data) != "inner" {
		t.Errorf("sub-root content = %q, want %q", data, "inner")
	}

	// top.txt lives in the parent, so it must not be visible from the sub-root.
	if _, err := sub.Stat("top.txt"); err == nil {
		t.Error("sub-root leaked parent file top.txt")
	}

	// Sub-root stays read-only.
	if _, err := sub.Create("new.txt"); err == nil {
		t.Error("sub-root Create unexpectedly succeeded; want read-only error")
	}
}

// TestFromIoFs_RoundTrip adapts an fs.FS into vroot via FromIoFs and back into
// fs.FS via ToIoFs, then validates the surface with fstest.TestFS. ToIoFs
// (not ToIoFsRoot) is used so it only needs the Fs[F] interface, sidestepping
// the unexported R type.
func TestFromIoFs_RoundTrip(t *testing.T) {
	dirFS := materializeDirFS(t, []string{
		`file1.txt: "f1"`,
		"subdir/",
		`subdir/nested.txt: "n"`,
	})
	r := vroot.FromIoFs(dirFS, "fs.FS")
	roundTripped := vroot.ToIoFs[vroot.File](r)

	if err := fstest.TestFS(roundTripped, "file1.txt", "subdir/nested.txt"); err != nil {
		t.Fatal(err)
	}
}

// TestFromIoFs_WritesRejected confirms the adapter is read-only.
func TestFromIoFs_WritesRejected(t *testing.T) {
	dirFS := materializeDirFS(t, []string{`f.txt: "x"`})
	r := vroot.FromIoFs(dirFS, "fs.FS")

	if _, err := r.Create("new.txt"); err == nil {
		t.Error("Create: want read-only error, got nil")
	}
	if err := r.Remove("f.txt"); err == nil {
		t.Error("Remove: want read-only error, got nil")
	}
	if err := r.Mkdir("d", 0o755); err == nil {
		t.Error("Mkdir: want read-only error, got nil")
	}
}
