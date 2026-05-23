package vroot_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func pathprefixOption() acceptancetest.Option {
	// Os left as zero (acceptancetest.OsEnv): auto-detect from runtime.GOOS.
	return acceptancetest.Option{
		SkipSymlink: runtime.GOOS == "windows" && os.Getenv("GITHUB_ACTIONS") != "true",
		SkipChown:   runtime.GOOS == "windows",
		ChownUid:    os.Getuid(),
		ChownGid:    os.Getgid(),
		// PathPrefixFs filepath.Clean-s before delegating, collapsing "tree/."
		// to "tree", so it cannot observe the trailing-dot case.
		SkipRemoveAllDotComponent: true,
	}
}

// TestPathPrefixFs_Acceptance runs the full Fs acceptance suite against an
// osfs.Fs exposed through PathPrefixFs so that a sub-directory ("prefix") of
// the temp dir acts as the root. Fixtures are materialized inside that
// sub-directory; every operation must behave as if "prefix" were the root.
func TestPathPrefixFs_Acceptance(t *testing.T) {
	opt := pathprefixOption()
	s := acceptancetest.Setup[*os.File, *vroot.PathPrefixFs[*os.File]]{
		Make: func(t *testing.T, lines []string) *vroot.PathPrefixFs[*os.File] {
			base := t.TempDir()
			prefixDir := filepath.Join(base, "prefix")
			if err := os.Mkdir(prefixDir, 0o755); err != nil {
				t.Fatalf("mkdir prefix: %v", err)
			}
			// Materialize fixtures inside the prefix dir.
			setupFs, err := osfs.NewFs(prefixDir)
			if err != nil {
				t.Fatalf("NewFs setup: %v", err)
			}
			testhelper.New[*testing.T, *os.File](t, setupFs).SetupLines(lines...)
			// Wrap the base Fs with the prefix so its root == prefixDir.
			baseFs, err := osfs.NewFs(base)
			if err != nil {
				t.Fatalf("NewFs base: %v", err)
			}
			return vroot.NewPathPrefixFs(baseFs, "prefix")
		},
		Option: opt,
	}
	acceptancetest.RunFs(t, s)
}

// TestPathPrefixFs_Scoping verifies operations land under the prefix in the
// underlying filesystem and that path traversal is rejected.
func TestPathPrefixFs_Scoping(t *testing.T) {
	base := t.TempDir()
	prefixDir := filepath.Join(base, "prefix")
	if err := os.Mkdir(prefixDir, 0o755); err != nil {
		t.Fatalf("mkdir prefix: %v", err)
	}
	// A sibling file outside the prefix that traversal might try to reach.
	if err := os.WriteFile(filepath.Join(base, "secret.txt"), []byte("s"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	baseFs, err := osfs.NewFs(base)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	w := vroot.NewPathPrefixFs(baseFs, "prefix")

	// A write through the wrapper lands under prefix/ in the real tree.
	if err := vroot.WriteFile(w, "hello.txt", []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(prefixDir, "hello.txt"))
	if err != nil {
		t.Fatalf("expected file under prefix: %v", err)
	}
	if string(got) != "hi" {
		t.Errorf("content = %q, want %q", got, "hi")
	}

	// Path traversal that would escape the prefix is rejected.
	for _, name := range []string{"../secret.txt", "../../etc/passwd", ".."} {
		if _, err := w.Open(name); !errors.Is(err, vroot.ErrPathEscapes) {
			t.Errorf("Open(%q): want ErrPathEscapes, got %v", name, err)
		}
		if _, err := w.Stat(name); !errors.Is(err, vroot.ErrPathEscapes) {
			t.Errorf("Stat(%q): want ErrPathEscapes, got %v", name, err)
		}
	}

	// The secret outside the prefix must not be reachable even though it
	// exists in the underlying tree.
	if _, err := w.Stat("secret.txt"); err == nil {
		t.Error("Stat(secret.txt) unexpectedly succeeded; prefix scoping leaked")
	}
}

// TestPathPrefixFs_Name checks Name reporting: prefix when set, inner Name
// when empty.
func TestPathPrefixFs_Name(t *testing.T) {
	base := t.TempDir()
	baseFs, err := osfs.NewFs(base)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}

	if got := vroot.NewPathPrefixFs(baseFs, "prefix").Name(); got != "prefix" {
		t.Errorf("Name() with prefix = %q, want %q", got, "prefix")
	}
	if got := vroot.NewPathPrefixFs(baseFs, "").Name(); got != baseFs.Name() {
		t.Errorf("Name() with empty prefix = %q, want inner Name %q", got, baseFs.Name())
	}
}

// TestPathPrefixFs_EmptyPrefix verifies an empty prefix behaves as a
// passthrough while still rejecting traversal.
func TestPathPrefixFs_EmptyPrefix(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	baseFs, err := osfs.NewFs(base)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	w := vroot.NewPathPrefixFs(baseFs, "")

	data, err := vroot.ReadFile(w, "f.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "x" {
		t.Errorf("content = %q, want %q", data, "x")
	}

	if _, err := w.Open("../outside"); !errors.Is(err, vroot.ErrPathEscapes) {
		t.Errorf("traversal with empty prefix: want ErrPathEscapes, got %v", err)
	}
}
