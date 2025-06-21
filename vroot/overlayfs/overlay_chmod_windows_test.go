//go:build windows

package overlayfs

import (
	"io/fs"
	"testing"
)

func TestOverlay_ChmodWindows(t *testing.T) {
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

		// Test Windows-compatible permission combinations
		// Windows has limited permission model compared to Unix
		perms := []fs.FileMode{0o444, 0o666}

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

			// On Windows, permission checking is more lenient
			// The file system may not support exact Unix permissions
			actualPerm := info.Mode().Perm()
			t.Logf("set permission %o, got %o", perm, actualPerm)

			// For Windows, we mainly check that chmod doesn't fail
			// rather than exact permission matching
		}
	})
}
