package vroot_test

import (
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/internal/prepare"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func TestToIoFsRooted(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	prepare.MakeFsys(tempDir, true, false)
	r, err := osfs.NewRooted(filepath.Join(tempDir, "root", "readable"))
	if err != nil {
		panic(err)
	}
	defer r.Close()
	fsys := vroot.ToIoFsRooted(r)
	fstest.TestFS(fsys, prepare.RootFsysReadableFiles...)
}

func TestToIoFsUnrooted(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	prepare.MakeFsys(tempDir, true, false)
	r, err := osfs.NewUnrooted(filepath.Join(tempDir, "root", "readable"))
	if err != nil {
		panic(err)
	}
	fsys := vroot.ToIoFsUnrooted(r)
	fstest.TestFS(fsys, prepare.RootFsysReadableFiles...)
}
