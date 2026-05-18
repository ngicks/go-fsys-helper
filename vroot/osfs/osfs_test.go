package osfs_test

import (
	"os"
	"runtime"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func hostOs() acceptancetest.Os {
	if runtime.GOOS == "windows" {
		return acceptancetest.OsWindows
	}
	return acceptancetest.OsUnix
}

func newOption() acceptancetest.Option {
	return acceptancetest.Option{
		Os:          hostOs(),
		SkipSymlink: runtime.GOOS == "windows" && os.Getenv("CLAUDE_TEST_SYMLINKS") != "1",
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
			return r
		},
		Option: opt,
	}
	acceptancetest.RunRoot(t, s)
}
