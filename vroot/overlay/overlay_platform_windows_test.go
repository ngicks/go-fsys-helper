//go:build windows

package overlay

import (
	"io/fs"
	"os"
	"testing"
)

func TestOverlay_WindowsSpecificBehavior(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)

	r := prepareLayers(tempDir)
	defer r.Close()

	t.Run("opened file cannot be removed on windows", func(t *testing.T) {
		// Create and open a file
		if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
			t.Fatal(err)
		}
		
		testFile, err := r.top.Create("root/writable/windows_test.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testFile.Write([]byte("test content")); err != nil {
			testFile.Close()
			t.Fatal(err)
		}
		testFile.Close()

		// Open file for reading
		f, err := r.OpenFile("root/writable/windows_test.txt", os.O_RDONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		// Try to remove the file while it's open - should fail on Windows
		err = r.Remove("root/writable/windows_test.txt")
		if err == nil {
			t.Error("expected error when removing opened file on Windows")
		}

		// File should still be visible
		_, err = r.Lstat("root/writable/windows_test.txt")
		if err != nil {
			t.Errorf("file should still exist after failed removal: %v", err)
		}
	})

	t.Run("opened file parent directory cannot be removed on windows", func(t *testing.T) {
		// Create directory and file
		if err := r.top.MkdirAll("root/writable/windir", fs.ModePerm); err != nil {
			t.Fatal(err)
		}
		
		testFile, err := r.top.Create("root/writable/windir/locked.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testFile.Write([]byte("test content")); err != nil {
			testFile.Close()
			t.Fatal(err)
		}
		testFile.Close()

		// Open file
		f, err := r.OpenFile("root/writable/windir/locked.txt", os.O_RDONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		// Try to remove parent directory - should fail on Windows
		err = r.RemoveAll("root/writable/windir")
		if err == nil {
			t.Error("expected error when removing directory with opened file on Windows")
		}

		// Directory should still exist
		_, err = r.Lstat("root/writable/windir")
		if err != nil {
			t.Errorf("directory should still exist after failed removal: %v", err)
		}
	})

	t.Run("case insensitive file operations", func(t *testing.T) {
		// Note: This test might not work on all Windows systems
		// depending on file system configuration
		if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
			t.Fatal(err)
		}

		// Create file with lowercase name
		testFile, err := r.top.Create("root/writable/lowercase.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testFile.Write([]byte("test content")); err != nil {
			testFile.Close()
			t.Fatal(err)
		}
		testFile.Close()

		// Try to access with different case - may work on Windows
		_, err = r.Lstat("root/writable/LOWERCASE.TXT")
		// Don't assert success/failure as this depends on file system config
		t.Logf("Case insensitive access result: %v", err)

		// Try to create file with different case
		_, err = r.Create("root/writable/LOWERCASE.TXT")
		if err == nil {
			t.Log("Windows allowed creating file with different case")
		} else {
			t.Logf("Windows prevented creating file with different case: %v", err)
		}
	})

	t.Run("windows path separators", func(t *testing.T) {
		// Test that Windows-style paths work
		if err := r.top.MkdirAll("root\\writable", fs.ModePerm); err != nil {
			// This might fail depending on how the overlay handles path separators
			t.Logf("Windows-style path creation failed: %v", err)
		}

		// Test mixed separators
		testFile, err := r.top.Create("root/writable\\mixed_separators.txt")
		if err != nil {
			t.Logf("Mixed separator path creation failed: %v", err)
		} else {
			testFile.Close()
			t.Log("Mixed separator path creation succeeded")
		}
	})

	t.Run("file locking behavior", func(t *testing.T) {
		// Create file
		if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
			t.Fatal(err)
		}
		
		testFile, err := r.top.Create("root/writable/lock_test.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testFile.Write([]byte("test content")); err != nil {
			testFile.Close()
			t.Fatal(err)
		}
		testFile.Close()

		// Open file for writing (exclusive on Windows)
		f1, err := r.OpenFile("root/writable/lock_test.txt", os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f1.Close()

		// Try to open same file for writing again - might fail on Windows
		f2, err := r.OpenFile("root/writable/lock_test.txt", os.O_RDWR, 0)
		if err != nil {
			t.Logf("Windows prevented second write handle: %v", err)
		} else {
			f2.Close()
			t.Log("Windows allowed second write handle")
		}

		// Try to open for reading - should generally work
		f3, err := r.OpenFile("root/writable/lock_test.txt", os.O_RDONLY, 0)
		if err != nil {
			t.Errorf("failed to open file for reading while write handle open: %v", err)
		} else {
			f3.Close()
		}
	})
}