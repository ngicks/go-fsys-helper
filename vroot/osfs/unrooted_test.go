package osfs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
)

func TestUnrooted(t *testing.T) {
	t.Run("with outside", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Logf("temp dir = %s", tempDir)
		acceptancetest.MakeOsFsys(tempDir, false, true)
		r, err := NewUnrooted(filepath.Join(tempDir, "root", "writable"))
		if err != nil {
			panic(err)
		}
		defer r.Close()
		acceptancetest.UnrootedReadWrite(t, r, true)
	})
	t.Run("without outside", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Logf("temp dir (no outside dir) = %s", tempDir)
		acceptancetest.MakeOsFsys(tempDir, false, true)
		_ = os.RemoveAll(filepath.Join(tempDir, "outside"))
		r, err := NewUnrooted(filepath.Join(tempDir, "root", "writable"))
		if err != nil {
			panic(err)
		}
		defer r.Close()
		acceptancetest.UnrootedReadWrite(t, r, false)
	})
}
