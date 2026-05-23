package vroot_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

// pathnormOption builds the acceptance Option for the pathnormalizer wrappers.
// Os is left as the zero value (acceptancetest.OsEnv) so the suite auto-detects
// unix/windows behavior from runtime.GOOS.
func pathnormOption() acceptancetest.Option {
	return acceptancetest.Option{
		SkipSymlink: runtime.GOOS == "windows" && os.Getenv("GITHUB_ACTIONS") != "true",
		SkipChown:   runtime.GOOS == "windows",
		ChownUid:    os.Getuid(),
		ChownGid:    os.Getgid(),
		// PathNormalizer/PathLocalizer filepath.Clean before delegating,
		// collapsing "tree/." to "tree", so the trailing-dot case is invisible.
		SkipRemoveAllDotComponent: true,
	}
}

// TestPathLocalizerRoot_Acceptance is the new-API analog of the old
// TestSepConverter/ToOsPath/acceptancetest subtest. It wraps an osfs.Root with
// PathLocalizerRoot (forward-slash external, OS-style internal) and runs the
// full Root acceptance suite through it.
func TestPathLocalizerRoot_Acceptance(t *testing.T) {
	opt := pathnormOption()
	s := acceptancetest.SetupRoot[*os.File, *vroot.PathLocalizerRoot[*os.File, *osfs.Root]]{
		Make: func(t *testing.T, lines []string) *vroot.PathLocalizerRoot[*os.File, *osfs.Root] {
			dir := t.TempDir()
			setupFs, err := osfs.NewFs(dir)
			if err != nil {
				t.Fatalf("NewFs setup: %v", err)
			}
			testhelper.New[*testing.T, *os.File](t, setupFs).SetupLines(lines...)
			r, err := osfs.NewRoot(dir)
			if err != nil {
				t.Fatalf("NewRoot: %v", err)
			}
			return vroot.NewPathLocalizerRoot(r)
		},
		Option: opt,
	}
	acceptancetest.RunRoot(t, s)
}

// TestPathLocalizerRoot_ForwardSlash mirrors the "accessing with slash
// separated path" sub-test of the old sepconverter_test.go: it verifies that
// directory enumeration through forward-slash paths resolves correctly when
// the underlying Root is OS-path-backed.
func TestPathLocalizerRoot_ForwardSlash(t *testing.T) {
	tempDir := t.TempDir()
	setupFs, err := osfs.NewFs(tempDir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	testhelper.New[*testing.T, *os.File](t, setupFs).SetupLines(
		"subdir/",
		"subdir/double_nested/",
		`subdir/file1.txt: "f1"`,
		`subdir/file2.txt: "f2"`,
		`subdir/double_nested/nested.txt: "n"`,
	)

	r, err := osfs.NewRoot(tempDir)
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}
	defer func() { _ = r.Close() }()

	localized := vroot.NewPathLocalizerRoot(r)

	assertNames := func(t *testing.T, want []string, got []fs.DirEntry) {
		t.Helper()
		names := make([]string, 0, len(got))
		for _, e := range got {
			names = append(names, e.Name())
		}
		slices.Sort(want)
		slices.Sort(names)
		if !slices.Equal(want, names) {
			t.Errorf("dir entries:\nwant: %v\ngot:  %v", want, names)
		}
	}

	// Trailing slash + forward-slash form, regardless of platform separator.
	entries, err := vroot.ReadDir(localized, "subdir/")
	if err != nil {
		t.Fatalf("ReadDir(subdir/): %v", err)
	}
	assertNames(t, []string{"double_nested", "file1.txt", "file2.txt"}, entries)

	entries, err = vroot.ReadDir(localized, "subdir/double_nested/")
	if err != nil {
		t.Fatalf("ReadDir(subdir/double_nested/): %v", err)
	}
	assertNames(t, []string{"nested.txt"}, entries)

	// Sanity: the same path with the platform separator also works (since
	// PathLocalizer uses filepath.Clean which accepts both).
	entries, err = vroot.ReadDir(localized, filepath.Join("subdir", "double_nested"))
	if err != nil {
		t.Fatalf("ReadDir(platform path): %v", err)
	}
	assertNames(t, []string{"nested.txt"}, entries)
}

// TestPathNormalizerRoot_RoundTrip composes PathNormalizer(PathLocalizer(osfs))
// to verify the converters cancel out: external is OS-style, the inner
// PathLocalizer sees forward slashes, and the osfs at the bottom sees OS
// paths again. On Linux the conversions are effectively no-ops (both
// separators are "/"), but the test still exercises every wrapper method.
func TestPathNormalizerRoot_RoundTrip(t *testing.T) {
	opt := pathnormOption()
	s := acceptancetest.SetupRoot[*os.File, *vroot.PathNormalizerRoot[*os.File, *vroot.PathLocalizerRoot[*os.File, *osfs.Root]]]{
		Make: func(t *testing.T, lines []string) *vroot.PathNormalizerRoot[*os.File, *vroot.PathLocalizerRoot[*os.File, *osfs.Root]] {
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
			localized := vroot.NewPathLocalizerRoot(inner)
			return vroot.NewPathNormalizerRoot(localized)
		},
		Option: opt,
	}
	acceptancetest.RunRoot(t, s)
}
