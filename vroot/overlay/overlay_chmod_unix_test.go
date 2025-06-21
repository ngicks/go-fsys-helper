//go:build unix

package overlay

import (
	"io/fs"
	"testing"
)

func TestOverlay_ChmodUnix(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)

	r, closers := prepareLayers(tempDir)
	defer r.Close()
	defer closers(t)

	t.Run("file permissions edge cases", func(t *testing.T) {
		if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
			t.Fatal(err)
		}

		f, err := r.top.Create("root/writable/perm_test.txt")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()

		// Test various permission combinations on Unix
		perms := []fs.FileMode{0o000, 0o444, 0o666, 0o755, 0o777}

		for _, perm := range perms {
			err = r.Chmod("root/writable/perm_test.txt", perm)
			if err != nil {
				t.Errorf("failed to set permission %o: %v", perm, err)
				continue
			}

			info, err := r.Lstat("root/writable/perm_test.txt")
			if err != nil {
				t.Fatal(err)
			}

			if info.Mode().Perm() != perm {
				t.Errorf("permission mismatch: expected %o, got %o", perm, info.Mode().Perm())
			}
		}
	})
}
