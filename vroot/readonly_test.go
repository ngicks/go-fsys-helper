package vroot_test

import (
	"errors"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func readonlyOption() acceptancetest.Option {
	// Os left as zero (acceptancetest.OsEnv): auto-detect from runtime.GOOS.
	return acceptancetest.Option{
		SkipSymlink: runtime.GOOS == "windows" && os.Getenv("GITHUB_ACTIONS") != "true",
		SkipChown:   runtime.GOOS == "windows",
		ChownUid:    os.Getuid(),
		ChownGid:    os.Getgid(),
	}
}

// TestReadOnlyRoot runs the read-only Root acceptance suite against an
// osfs.Root wrapped in ReadOnlyRoot. Write methods on the wrapper return
// errdef.EROFS; the read-only suite never exercises them, so all sub-tests
// should pass against the wrapped Root.
func TestReadOnlyRoot(t *testing.T) {
	opt := readonlyOption()
	s := acceptancetest.SetupRoot[*vroot.ReadOnlyFile, *vroot.ReadOnlyRoot[*os.File, *osfs.Root]]{
		Make: func(t *testing.T, lines []string) *vroot.ReadOnlyRoot[*os.File, *osfs.Root] {
			dir := t.TempDir()
			setupFs, err := osfs.NewFs(dir)
			if err != nil {
				t.Fatalf("NewFs setup: %v", err)
			}
			testhelper.New[*testing.T, *os.File](t, setupFs).SetupLines(lines...)
			inner, err := osfs.NewRoot(dir)
			if err != nil {
				t.Fatalf("NewRoot: %v", err)
			}
			return vroot.NewReadOnlyRoot(inner)
		},
		Option: opt,
	}
	acceptancetest.RunRootReadOnly(t, s)
}

// TestReadOnlyFs runs the read-only Fs acceptance suite against an osfs.Fs
// wrapped in ReadOnlyFs.
func TestReadOnlyFs(t *testing.T) {
	opt := readonlyOption()
	s := acceptancetest.Setup[*vroot.ReadOnlyFile, *vroot.ReadOnlyFs[*os.File]]{
		Make: func(t *testing.T, lines []string) *vroot.ReadOnlyFs[*os.File] {
			dir := t.TempDir()
			setupFs, err := osfs.NewFs(dir)
			if err != nil {
				t.Fatalf("NewFs setup: %v", err)
			}
			testhelper.New[*testing.T, *os.File](t, setupFs).SetupLines(lines...)
			inner, err := osfs.NewFs(dir)
			if err != nil {
				t.Fatalf("NewFs: %v", err)
			}
			return vroot.NewReadOnlyFs[*os.File](inner)
		},
		Option: opt,
	}
	acceptancetest.RunFsReadOnly(t, s)
}

// TestReadOnly_WritesRejected explicitly verifies that every write method
// returns an EROFS-wrapped error. The acceptance suite covers reads; this
// pins the "all writes fail" contract separately.
func TestReadOnly_WritesRejected(t *testing.T) {
	tempDir := t.TempDir()
	setupFs, err := osfs.NewFs(tempDir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	testhelper.New[*testing.T, *os.File](t, setupFs).SetupLines(
		`f.txt: "hello"`,
	)
	inner, err := osfs.NewRoot(tempDir)
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}
	defer func() { _ = inner.Close() }()
	ro := vroot.NewReadOnlyRoot(inner)

	check := func(t *testing.T, op string, err error) {
		t.Helper()
		if err == nil {
			t.Errorf("%s: want error, got nil", op)
			return
		}
		if !errors.Is(err, errdef.EROFS) {
			t.Errorf("%s: want EROFS, got %v", op, err)
		}
	}

	check(t, "Chmod", ro.Chmod("f.txt", 0o600))
	check(t, "Chown", ro.Chown("f.txt", 0, 0))
	check(t, "Chtimes", ro.Chtimes("f.txt", time.Now(), time.Now()))
	_, createErr := ro.Create("new.txt")
	check(t, "Create", createErr)
	check(t, "Lchown", ro.Lchown("f.txt", 0, 0))
	check(t, "Link", ro.Link("f.txt", "f2.txt"))
	check(t, "Mkdir", ro.Mkdir("dir", 0o755))
	check(t, "MkdirAll", ro.MkdirAll("dir", 0o755))
	check(t, "Remove", ro.Remove("f.txt"))
	check(t, "RemoveAll", ro.RemoveAll("f.txt"))
	check(t, "Rename", ro.Rename("f.txt", "f2.txt"))
	check(t, "Symlink", ro.Symlink("f.txt", "link"))

	// OpenFile with a write flag must also fail.
	_, openErr := ro.OpenFile("f.txt", os.O_WRONLY, 0)
	check(t, "OpenFile(write)", openErr)

	// Sanity: read operations on the same Root work.
	if _, err := ro.Stat("f.txt"); err != nil {
		t.Errorf("Stat: %v", err)
	}
	if _, err := vroot.ReadFile(ro, "f.txt"); err != nil {
		t.Errorf("ReadFile: %v", err)
	}
}
