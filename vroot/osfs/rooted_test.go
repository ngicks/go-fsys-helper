package osfs

import (
	"path/filepath"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
)

func TestRooted(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	acceptancetest.MakeFsys(tempDir, false, true)
	r, err := NewRooted(filepath.Join(tempDir, "root", "writable"))
	if err != nil {
		panic(err)
	}
	defer r.Close()
	acceptancetest.RootedReadWrite(t, r)
}
