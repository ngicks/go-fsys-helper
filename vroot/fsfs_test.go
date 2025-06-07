package vroot_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/internal/prepare"
)

func Test(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	prepare.MakeFsys(tempDir, true, false)
	fsys := os.DirFS(filepath.Join(tempDir, "root", "readable"))
	r := vroot.NewFsRooted(fsys.(fs.ReadLinkFS), "fs.FS")
	acceptancetest.RootedReadOnly(t, r)
}

func TestFsUnrooted(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	prepare.MakeFsys(tempDir, true, false)
	fsys := os.DirFS(filepath.Join(tempDir, "root", "readable"))
	u := vroot.NewFsUnrooted(fsys.(fs.ReadLinkFS), "fs.FS")
	acceptancetest.UnrootedReadOnly(t, u, true)
}
