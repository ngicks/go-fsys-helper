package osfs_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func newOption() acceptancetest.Option {
	// Os is left as the zero value (acceptancetest.OsEnv) so the suite
	// auto-detects unix/windows behavior from runtime.GOOS.
	return acceptancetest.Option{
		SkipSymlink: runtime.GOOS == "windows" && os.Getenv("GITHUB_ACTIONS") != "true",
		SkipChown:   runtime.GOOS == "windows",
		ChownUid:    os.Getuid(),
		ChownGid:    os.Getgid(),
	}
}

func TestFs(t *testing.T) {
	opt := newOption()
	s := acceptancetest.Setup[*os.File, *osfs.Fs]{
		Make: func(t *testing.T, lines []string) *osfs.Fs {
			dir := t.TempDir()
			setupFs, err := osfs.NewFs(dir)
			if err != nil {
				t.Fatalf("NewFs setup: %v", err)
			}
			testhelper.New(t, setupFs).SetupLines(lines...)
			fsys, err := osfs.NewFs(dir)
			if err != nil {
				t.Fatalf("NewFs: %v", err)
			}
			return fsys
		},
		Option: opt,
	}
	acceptancetest.RunFs(t, s)
}

func TestRoot(t *testing.T) {
	opt := newOption()
	s := acceptancetest.SetupRoot[*os.File, *osfs.Root]{
		Make: func(t *testing.T, lines []string) *osfs.Root {
			// Root the Fs at base/root and create genuine out-of-root targets
			// as siblings of root (base/outside, base/outsidedir/...). The
			// acceptance escape suite points its "../outside" / "../outsidedir"
			// symlinks at these, so a regression that follows an escaping
			// symlink reads real bytes here instead of a benign ErrNotExist.
			base := t.TempDir()
			root := filepath.Join(base, "root")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatalf("mkdir root: %v", err)
			}
			makeEscapeTargets(t, base)
			setupFs, err := osfs.NewFs(root)
			if err != nil {
				t.Fatalf("NewFs setup: %v", err)
			}
			testhelper.New[*testing.T, *os.File](t, setupFs).SetupLines(lines...)
			r, err := osfs.NewRoot(root)
			if err != nil {
				t.Fatalf("NewRoot: %v", err)
			}
			return r
		},
		Option: opt,
	}
	acceptancetest.RunRoot(t, s)
}

// makeEscapeTargets writes the genuine out-of-root files the acceptance escape
// suite's "../outside" / "../outsidedir" symlinks resolve to. They contain a
// recognizable marker so a confinement regression that actually reads them is
// obvious. base is the parent of the Fs root; "../outside" from inside the root
// resolves to base/outside.
func makeEscapeTargets(t *testing.T, base string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(base, "outside"), []byte("OUT-OF-ROOT-SECRET"), 0o644); err != nil {
		t.Fatalf("write outside target: %v", err)
	}
	if err := os.Mkdir(filepath.Join(base, "outsidedir"), 0o755); err != nil {
		t.Fatalf("mkdir outsidedir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(base, "outsidedir", "inside"), []byte("OUT-OF-ROOT-SECRET-2"), 0o644); err != nil {
		t.Fatalf("write outsidedir target: %v", err)
	}
}
