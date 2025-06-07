package vroot_test

import (
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
	r, err := vroot.NewFsRooted(fsys, "fs.FS")
	if err != nil {
		panic(err)
	}
	acceptancetest.RootedReadOnly(t, r)
}
