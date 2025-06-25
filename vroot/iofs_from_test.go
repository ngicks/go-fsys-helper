package vroot_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
)

func TestFromIoFsRooted(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	acceptancetest.MakeOsFsys(tempDir, true, false)
	fsys := os.DirFS(filepath.Join(tempDir, "root", "readable"))
	r := vroot.FromIoFsRooted(fsys.(fs.ReadLinkFS), "fs.FS")
	acceptancetest.RootedReadOnly(t, r)
	fstest.TestFS(vroot.ToIoFsRooted(r), acceptancetest.RootFsysReadableFiles...)
}

func TestFromIoFsUnrooted(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	acceptancetest.MakeOsFsys(tempDir, true, false)
	fsys := os.DirFS(filepath.Join(tempDir, "root", "readable"))
	u := vroot.FromIoFsUnrooted(fsys.(fs.ReadLinkFS), "fs.FS")
	acceptancetest.UnrootedReadOnly(t, u, true)
	fstest.TestFS(vroot.ToIoFsUnrooted(u), acceptancetest.RootFsysReadableFiles...)
}
