package vroot_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
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
	}
}

// TestPathPrefixFs_Acceptance runs the full Fs acceptance suite against an
// osfs.Fs exposed through PathPrefixFs so that a sub-directory ("prefix") of
// the temp dir acts as the root. Fixtures are materialized inside that
// sub-directory; every operation must behave as if "prefix" were the root.
func TestPathPrefixFs_Acceptance(t *testing.T) {
	opt := pathprefixOption()
	s := acceptancetest.Setup[*osfs.File, *vroot.PathPrefixFs[*osfs.File]]{
		Make: func(t *testing.T, lines []string) *vroot.PathPrefixFs[*osfs.File] {
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
			testhelper.New[*testing.T, *osfs.File](t, setupFs).SetupLines(lines...)
			// Wrap the base Fs with the prefix so its root == prefixDir.
			baseFs, err := osfs.NewFs(base)
			if err != nil {
				t.Fatalf("NewFs base: %v", err)
			}
			w, err := vroot.NewPathPrefixFs(baseFs, "prefix")
			if err != nil {
				t.Fatalf("NewPathPrefixFs: %v", err)
			}
			return w
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
	w, err := vroot.NewPathPrefixFs(baseFs, "prefix")
	if err != nil {
		t.Fatalf("NewPathPrefixFs: %v", err)
	}

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

// TestPathPrefixFs_Name checks Name reports "prefix=<prefix>: <inner name>".
func TestPathPrefixFs_Name(t *testing.T) {
	base := t.TempDir()
	if err := os.Mkdir(filepath.Join(base, "prefix"), 0o755); err != nil {
		t.Fatalf("mkdir prefix: %v", err)
	}
	baseFs, err := osfs.NewFs(base)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}

	w, err := vroot.NewPathPrefixFs(baseFs, "prefix")
	if err != nil {
		t.Fatalf("NewPathPrefixFs: %v", err)
	}
	want := "prefix=prefix: " + baseFs.Name()
	if got := w.Name(); got != want {
		t.Errorf("Name() = %q, want %q", got, want)
	}
}

// TestPathPrefixFs_PrefixValidation verifies the constructor validates the
// prefix up front (like osfs.NewFs): empty -> fs.ErrInvalid, non-existent ->
// fs.ErrNotExist, regular file -> ENOTDIR.
func TestPathPrefixFs_PrefixValidation(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "file.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	baseFs, err := osfs.NewFs(base)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}

	if _, err := vroot.NewPathPrefixFs(baseFs, ""); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf(`NewPathPrefixFs(_, ""): want fs.ErrInvalid, got %v`, err)
	}
	if _, err := vroot.NewPathPrefixFs(baseFs, "nonexistent"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf(`NewPathPrefixFs(_, "nonexistent"): want fs.ErrNotExist, got %v`, err)
	}
	if _, err := vroot.NewPathPrefixFs(baseFs, "file.txt"); !errors.Is(err, syscall.ENOTDIR) {
		t.Errorf(`NewPathPrefixFs(_, "file.txt"): want ENOTDIR, got %v`, err)
	}
}
