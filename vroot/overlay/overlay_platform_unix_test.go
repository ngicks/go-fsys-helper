//go:build unix

package overlay

import (
	"io/fs"
	"os"
	"syscall"
	"testing"
)

func TestOverlay_UnixSpecificBehavior(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)

	r := prepareLayers(tempDir)
	defer r.Close()

	t.Run("opened file can be removed on unix", func(t *testing.T) {
		// Create and open a file
		if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
			t.Fatal(err)
		}

		testFile, err := r.top.Create("root/writable/unix_test.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testFile.Write([]byte("test content")); err != nil {
			testFile.Close()
			t.Fatal(err)
		}
		testFile.Close()

		// Open file for reading
		f, err := r.OpenFile("root/writable/unix_test.txt", os.O_RDONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		// Remove the file while it's open - should succeed on Unix
		err = r.Remove("root/writable/unix_test.txt")
		if err != nil {
			t.Errorf("failed to remove opened file on Unix: %v", err)
		}

		// File should no longer be visible in directory listing
		_, err = r.Lstat("root/writable/unix_test.txt")
		if err == nil {
			t.Error("removed file should not be visible in directory")
		}

		// But opened file handle should still be readable
		buf := make([]byte, 12)
		n, err := f.Read(buf)
		if err != nil {
			t.Errorf("failed to read from opened file after removal: %v", err)
		}
		if string(buf[:n]) != "test content" {
			t.Errorf("unexpected content: %s", string(buf[:n]))
		}
	})

	t.Run("file permissions and ownership", func(t *testing.T) {
		// Create file with specific permissions
		if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
			t.Fatal(err)
		}

		testFile, err := r.top.Create("root/writable/perm_test.txt")
		if err != nil {
			t.Fatal(err)
		}
		testFile.Close()

		// Set specific permissions through overlay
		err = r.Chmod("root/writable/perm_test.txt", 0o644)
		if err != nil {
			t.Errorf("failed to chmod through overlay: %v", err)
		}

		// Verify permissions
		info, err := r.Lstat("root/writable/perm_test.txt")
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Errorf("expected permissions 0644, got %o", info.Mode().Perm())
		}

		// Test chown if running as root (otherwise skip)
		if os.Getuid() == 0 {
			err = r.Chown("root/writable/perm_test.txt", 1000, 1000)
			if err != nil {
				t.Errorf("failed to chown through overlay: %v", err)
			}
		}
	})

	t.Run("hard links behavior", func(t *testing.T) {
		// Create file in top layer
		if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
			t.Fatal(err)
		}

		testFile, err := r.top.Create("root/writable/original.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testFile.Write([]byte("original content")); err != nil {
			testFile.Close()
			t.Fatal(err)
		}
		testFile.Close()

		// Create hard link
		err = r.Link("root/writable/original.txt", "root/writable/hardlink.txt")
		if err != nil {
			t.Errorf("failed to create hard link: %v", err)
		}

		// Both files should have same content
		content1, err := readFileContent(r, "root/writable/original.txt")
		if err != nil {
			t.Fatal(err)
		}
		content2, err := readFileContent(r, "root/writable/hardlink.txt")
		if err != nil {
			t.Fatal(err)
		}
		if content1 != content2 {
			t.Errorf("hard linked files have different content: %s vs %s", content1, content2)
		}

		// Check that they have the same inode (Unix-specific)
		info1, err := r.Lstat("root/writable/original.txt")
		if err != nil {
			t.Fatal(err)
		}
		info2, err := r.Lstat("root/writable/hardlink.txt")
		if err != nil {
			t.Fatal(err)
		}

		stat1 := info1.Sys().(*syscall.Stat_t)
		stat2 := info2.Sys().(*syscall.Stat_t)
		if stat1.Ino != stat2.Ino {
			t.Error("hard linked files should have same inode")
		}
	})

	t.Run("symlink behavior with relative paths", func(t *testing.T) {
		// Create target file and symlink in top layer
		if err := r.top.MkdirAll("root/writable/subdir", fs.ModePerm); err != nil {
			t.Fatal(err)
		}

		testFile, err := r.top.Create("root/writable/subdir/target.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testFile.Write([]byte("target content")); err != nil {
			testFile.Close()
			t.Fatal(err)
		}
		testFile.Close()

		// Create relative symlink (should point to subdir/target.txt from writable directory)
		err = r.Symlink("subdir/target.txt", "root/writable/relative_link")
		if err != nil {
			t.Errorf("failed to create relative symlink: %v", err)
		}

		// Follow symlink and read content
		content, err := readFileContent(r, "root/writable/relative_link")
		if err != nil {
			t.Errorf("failed to read through relative symlink: %v", err)
		}
		if content != "target content" {
			t.Errorf("unexpected content through symlink: %s", content)
		}

		// Verify symlink target
		target, err := r.ReadLink("root/writable/relative_link")
		if err != nil {
			t.Errorf("failed to read symlink target: %v", err)
		}
		if target != "subdir/target.txt" {
			t.Errorf("unexpected symlink target: %s", target)
		}
	})
}

func readFileContent(r *Overlay, path string) (string, error) {
	f, err := r.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf := make([]byte, 1024)
	n, err := f.Read(buf)
	if err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}
