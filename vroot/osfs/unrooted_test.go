package vroot

import (
	"path/filepath"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot/acceptancetest"
)

func TestUnrooted(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)
	makeFsys(tempDir, false)
	r, err := NewUnrooted(filepath.Join(tempDir, "root", "writable"))
	if err != nil {
		panic(err)
	}
	acceptancetest.UnrootedReadWrite(t, r, true)
}
