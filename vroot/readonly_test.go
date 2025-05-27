package vroot_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
	"github.com/ngicks/go-fsys-helper/vroot/internal/prepare"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func TestReadOnlyRooted(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	prepare.MakeFsys(tempDir, true, false)
	r, err := osfs.NewRooted(filepath.Join(tempDir, "root", "readable"))
	if err != nil {
		panic(err)
	}
	readonly := vroot.NewReadOnlyRooted(r)
	acceptancetest.RootedReadOnly(t, readonly)
}

func TestReadOnlyUnrooted(t *testing.T) {
	t.Run("with outside", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Logf("temp dir = %s", tempDir)
		prepare.MakeFsys(tempDir, true, false)
		r, err := osfs.NewUnrooted(filepath.Join(tempDir, "root", "readable"))
		if err != nil {
			panic(err)
		}
		readonly := vroot.NewReadOnlyUnrooted(r)
		acceptancetest.UnrootedReadOnly(t, readonly, true)
	})
	t.Run("without outside", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Logf("temp dir (no outside dir) = %s", tempDir)
		prepare.MakeFsys(tempDir, true, false)
		_ = os.RemoveAll(filepath.Join(tempDir, "outside"))
		r, err := osfs.NewUnrooted(filepath.Join(tempDir, "root", "readable"))
		if err != nil {
			panic(err)
		}
		readonly := vroot.NewReadOnlyUnrooted(r)
		acceptancetest.UnrootedReadOnly(t, readonly, false)
	})
}
