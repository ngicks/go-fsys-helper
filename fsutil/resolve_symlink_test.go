package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

type osfsLite struct {
	base string
}

func (fsys osfsLite) ReadLink(name string) (string, error) {
	return os.Readlink(filepath.Join(fsys.base, name))
}

func (fsys osfsLite) Lstat(name string) (fs.FileInfo, error) {
	return os.Lstat(filepath.Join(fsys.base, name))
}

func Test_resolveSymlink(t *testing.T) {
	tempDir := t.TempDir()

	// You can't use prepare.ExecuteLines since
	// it will cause cyclic import
	var err error
	err = os.MkdirAll(filepath.Join(tempDir, "root"), fs.ModePerm)
	if err != nil {
		panic(err)
	}

	err = os.Symlink("./a", filepath.Join(tempDir, "root", "b"))
	if err != nil {
		panic(err)
	}
	err = os.Symlink("./b", filepath.Join(tempDir, "root", "a"))
	if err != nil {
		panic(err)
	}

	fsys := osfsLite{filepath.Join(tempDir, "root")}
	resolved, err := ResolveSymlink(fsys, "a")
	if resolved != "" || !errors.Is(err, syscall.ELOOP) {
		t.Errorf(
			"incorrect: expected \"\" and error that satisfies erorrs.Is(err, syscall.ELOOP), but is %q and %v",
			resolved, err,
		)
	}
}
