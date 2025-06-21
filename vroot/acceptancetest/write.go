package acceptancetest

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// Main write function that orchestrates all write tests
func write(t *testing.T, fsys vroot.Fs) {
	t.Run("create fails without parent dirs", func(t *testing.T) {
		testCreateFailsWithoutParentDirs(t, fsys)
	})
	t.Run("create and write", func(t *testing.T) {
		testCreateAndWrite(t, fsys)
	})
	t.Run("file chmod", func(t *testing.T) {
		testFileChmod(t, fsys)
	})
	t.Run("openfile write and truncate", func(t *testing.T) {
		testOpenFileWriteAndTruncate(t, fsys)
	})
	t.Run("filesystem chmod", func(t *testing.T) {
		testFilesystemChmod(t, fsys)
	})
	t.Run("chtimes", func(t *testing.T) {
		testChtimes(t, fsys)
	})
	t.Run("mkdir", func(t *testing.T) {
		testMkdir(t, fsys)
	})
	t.Run("symlink", func(t *testing.T) {
		testSymlink(t, fsys)
	})
	t.Run("link", func(t *testing.T) {
		testLink(t, fsys)
	})
	t.Run("rename", func(t *testing.T) {
		testRename(t, fsys)
	})
	t.Run("path normalization", func(t *testing.T) {
		testPathNormalization(t, fsys)
	})
	t.Run("remove", func(t *testing.T) {
		testRemove(t, fsys)
	})
	t.Run("removeall", func(t *testing.T) {
		testRemoveAll(t, fsys)
	})
}

// Test that Create and OpenFile fail when parent directories don't exist
func testCreateFailsWithoutParentDirs(t *testing.T, fsys vroot.Fs) {
	// Test that Create fails when parent directories don't exist
	f, err := fsys.Create(filepath.FromSlash("nonexistent/dir/test.txt"))
	if err == nil {
		f.Close()
		t.Error("Create should fail when parent directories don't exist")
	}

	// Test that OpenFile with O_CREATE fails when parent directories don't exist
	f, err = fsys.OpenFile(filepath.FromSlash("another/nonexistent/path/test.txt"), os.O_CREATE|os.O_RDWR, 0o644)
	if err == nil {
		f.Close()
		t.Error("OpenFile with O_CREATE should fail when parent directories don't exist")
	}
}

// Test basic file creation and writing
func testCreateAndWrite(t *testing.T, fsys vroot.Fs) {
	// Create a new file
	f, err := fsys.Create("test_write.txt")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer f.Close()

	// Write to the file
	content := "test content"
	n, err := f.Write([]byte(content))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(content) {
		t.Fatalf("Write wrote %d bytes, expected %d", n, len(content))
	}

	// Close and verify Write effect by reopening and reading
	f.Close()
	f2, err := fsys.Open("test_write.txt")
	if err != nil {
		t.Fatalf("Reopen test_write.txt failed: %v", err)
	}
	defer f2.Close()

	readBuf, err := io.ReadAll(f2)
	if err != nil {
		t.Fatalf("ReadAll after Write failed: %v", err)
	}
	if string(readBuf) != content {
		t.Errorf("Write effect not observed: got %q, expected %q", string(readBuf), content)
	}
}

// testFileChmod is implemented in platform-specific files:
// - write_unix.go for Unix systems
// - write_windows.go for Windows systems

// Test OpenFile with write flags, WriteString, and Truncate
func testOpenFileWriteAndTruncate(t *testing.T, fsys vroot.Fs) {
	// Test OpenFile with write flags
	f, err := fsys.OpenFile("test_write_string.txt", os.O_RDWR|os.O_CREATE, 0o755)
	if err != nil {
		t.Fatalf("OpenFile with O_RDWR failed: %v", err)
	}
	defer f.Close()

	// Write using WriteString
	stringContent := "test string"
	n, err := f.WriteString(stringContent)
	if err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}
	if n != len(stringContent) {
		t.Fatalf("WriteString wrote %d bytes, expected %d", n, len(stringContent))
	}

	// Test Truncate
	err = f.Truncate(4)
	if err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	// Verify WriteString and Truncate effects
	f.Close()
	f2, err := fsys.Open("test_write_string.txt")
	if err != nil {
		t.Fatalf("Reopen test_write_string.txt failed: %v", err)
	}
	defer f2.Close()

	truncatedBuf, err := io.ReadAll(f2)
	if err != nil {
		t.Fatalf("ReadAll after WriteString and Truncate failed: %v", err)
	}
	expectedTruncated := stringContent[:4] // "test"
	if len(truncatedBuf) != 4 || string(truncatedBuf) != expectedTruncated {
		t.Errorf("WriteString/Truncate effect not observed: got %q (len=%d), expected %q (len=4)",
			string(truncatedBuf), len(truncatedBuf), expectedTruncated)
	}
}

// testFilesystemChmod is implemented in platform-specific files:
// - write_unix.go for Unix systems
// - write_windows.go for Windows systems

// Test Chtimes operations
func testChtimes(t *testing.T, fsys vroot.Fs) {
	// Create a file for testing
	f, err := fsys.Create("test_chtimes.txt")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	f.Close()

	oldTime := time.Now().Add(-time.Hour)        // Set to 1 hour ago
	newTime := time.Now().Add(-30 * time.Minute) // Set to 30 minutes ago
	err = fsys.Chtimes("test_chtimes.txt", oldTime, newTime)
	if err != nil {
		t.Fatalf("Chtimes failed: %v", err)
	}

	// Verify Chtimes effect
	info, err := fsys.Stat("test_chtimes.txt")
	if err != nil {
		t.Fatalf("Stat after Chtimes failed: %v", err)
	}
	// Check if modification time is reasonably close (within 1 millisecond)
	if info.ModTime().Sub(newTime).Abs() > time.Millisecond {
		t.Errorf("Chtimes effect not observed: got modtime %v, expected around %v", info.ModTime(), newTime)
	}
}

// testMkdir is implemented in platform-specific files:
// - write_unix.go for Unix systems
// - write_windows.go for Windows systems

// Test symlink creation and verification
func testSymlink(t *testing.T, fsys vroot.Fs) {
	// Create a target file first
	f, err := fsys.Create("symlink_target.txt")
	if err != nil {
		t.Fatalf("Create target file failed: %v", err)
	}
	f.Close()

	// Test Symlink (create a symlink that doesn't escape)
	err = fsys.Symlink("symlink_target.txt", "test_symlink")
	if err != nil {
		t.Fatalf("Symlink failed: %v", err)
	}

	// Verify Symlink effect
	info, err := fsys.Lstat("test_symlink")
	if err != nil {
		t.Fatalf("Lstat after Symlink failed: %v", err)
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("Symlink effect not observed: created item is not a symlink")
	}
	target, err := fsys.ReadLink("test_symlink")
	if err != nil {
		t.Fatalf("ReadLink after Symlink failed: %v", err)
	}
	if target != "symlink_target.txt" {
		t.Errorf("Symlink target not correct: got %q, expected %q", target, "symlink_target.txt")
	}
}

// Test hard link creation and verification
func testLink(t *testing.T, fsys vroot.Fs) {
	// Create a target file with content
	f, err := fsys.Create("link_target.txt")
	if err != nil {
		t.Fatalf("Create target file failed: %v", err)
	}
	content := "link test content"
	f.Write([]byte(content))
	f.Close()

	// Test Link (hard link)
	err = fsys.Link("link_target.txt", "test_hardlink")
	if err != nil {
		t.Fatalf("Link failed: %v", err)
	}

	// Verify Link effect
	info, err := fsys.Stat("test_hardlink")
	if err != nil {
		t.Fatalf("Stat after Link failed: %v", err)
	}
	if info.IsDir() {
		t.Errorf("Link effect not observed: hard link appears as directory")
	}

	// Verify content is the same
	f2, err := fsys.Open("test_hardlink")
	if err != nil {
		t.Fatalf("Open hardlink failed: %v", err)
	}
	defer f2.Close()
	linkBuf, err := io.ReadAll(f2)
	if err != nil {
		t.Fatalf("ReadAll hardlink failed: %v", err)
	}
	if string(linkBuf) != content {
		t.Errorf("Link content not correct: got %q, expected %q", string(linkBuf), content)
	}

	// Test that hard links share the same data by writing to the hard link
	// and verifying the write propagates to the original file
	f3, err := fsys.OpenFile("test_hardlink", os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatalf("OpenFile hardlink for append failed: %v", err)
	}
	appendedData := " appended"
	_, err = f3.Write([]byte(appendedData))
	if err != nil {
		t.Fatalf("Write to hardlink failed: %v", err)
	}
	f3.Close()

	// Read from the original file to verify the write propagated
	f4, err := fsys.Open("link_target.txt")
	if err != nil {
		t.Fatalf("Open original file after hardlink write failed: %v", err)
	}
	defer f4.Close()

	updatedBuf, err := io.ReadAll(f4)
	if err != nil {
		t.Fatalf("ReadAll original file after hardlink write failed: %v", err)
	}
	expectedUpdated := content + appendedData
	if string(updatedBuf) != expectedUpdated {
		t.Errorf("Hard link write propagation failed: got %q, expected %q", string(updatedBuf), expectedUpdated)
	}
}

// Test rename operations
func testRename(t *testing.T, fsys vroot.Fs) {
	// Create a file to rename
	f, err := fsys.Create("test_rename_source.txt")
	if err != nil {
		t.Fatalf("Create source file failed: %v", err)
	}
	f.Close()

	// Test Rename
	err = fsys.Rename("test_rename_source.txt", "test_renamed.txt")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	// Verify Rename effect
	_, err = fsys.Stat("test_rename_source.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Rename effect not observed: old file still exists")
	}
	info, err := fsys.Stat("test_renamed.txt")
	if err != nil {
		t.Fatalf("Stat after Rename failed: %v", err)
	}
	if info.IsDir() {
		t.Errorf("Renamed file appears as directory")
	}
}

// Test path normalization - "./filename" and "filename" should refer to the same file
// afero's in-mem fsys accounted those 2 differently and was causing terrible trouble.
// This test just sits here to prevent it from happening again.
func testPathNormalization(t *testing.T, fsys vroot.Fs) {
	// Create a file with "./" prefix
	f1, err := fsys.Create(filepath.FromSlash("./with_dot.txt"))
	if err != nil {
		t.Fatalf("Create ./with_dot.txt failed: %v", err)
	}
	f1.Close()

	// Create a file without "./" prefix
	f2, err := fsys.Create("without_dot.txt")
	if err != nil {
		t.Fatalf("Create without_dot.txt failed: %v", err)
	}
	f2.Close()

	// Test that both path forms can be accessed via Stat
	_, err = fsys.Stat("with_dot.txt") // without "./"
	if err != nil {
		t.Errorf("Stat with_dot.txt (without ./) failed: %v", err)
	}

	_, err = fsys.Stat(filepath.FromSlash("./with_dot.txt")) // with "./"
	if err != nil {
		t.Errorf("Stat ./with_dot.txt (with ./) failed: %v", err)
	}

	_, err = fsys.Stat("without_dot.txt") // without "./"
	if err != nil {
		t.Errorf("Stat without_dot.txt (without ./) failed: %v", err)
	}

	_, err = fsys.Stat(filepath.FromSlash("./without_dot.txt")) // with "./"
	if err != nil {
		t.Errorf("Stat ./without_dot.txt (with ./) failed: %v", err)
	}
}

// Test Remove operations on files and directories
func testRemove(t *testing.T, fsys vroot.Fs) {
	// Create a file to remove
	f, err := fsys.Create("test_remove_file.txt")
	if err != nil {
		t.Fatalf("Create file for removal failed: %v", err)
	}
	f.Write([]byte("content to be removed"))
	f.Close()

	// Test removing a file
	err = fsys.Remove("test_remove_file.txt")
	if err != nil {
		t.Fatalf("Remove file failed: %v", err)
	}

	// Verify file is gone
	_, err = fsys.Stat("test_remove_file.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Remove file effect not observed: file still exists or wrong error: %v", err)
	}

	// Create an empty directory to remove
	err = fsys.Mkdir("test_remove_empty_dir", 0o755)
	if err != nil {
		t.Fatalf("Mkdir for removal failed: %v", err)
	}

	// Test removing an empty directory
	err = fsys.Remove("test_remove_empty_dir")
	if err != nil {
		t.Fatalf("Remove empty directory failed: %v", err)
	}

	// Verify directory is gone
	_, err = fsys.Stat("test_remove_empty_dir")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Remove directory effect not observed: directory still exists or wrong error: %v", err)
	}

	// Create a non-empty directory
	err = fsys.Mkdir("test_remove_nonempty_dir", 0o755)
	if err != nil {
		t.Fatalf("Mkdir for non-empty test failed: %v", err)
	}
	f2, err := fsys.Create(filepath.FromSlash("test_remove_nonempty_dir/file.txt"))
	if err != nil {
		t.Fatalf("Create file in directory failed: %v", err)
	}
	f2.Close()

	// Test that removing non-empty directory fails
	err = fsys.Remove("test_remove_nonempty_dir")
	if err == nil {
		t.Error("Remove should fail on non-empty directory")
	}

	// Test removing non-existent file
	err = fsys.Remove("non_existent_file.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Remove non-existent file should return ErrNotExist, got: %v", err)
	}

	// Create a symlink and test removing it
	err = fsys.Symlink("target.txt", "test_remove_symlink")
	if err != nil {
		t.Fatalf("Create symlink for removal failed: %v", err)
	}

	// Remove the symlink (not the target)
	err = fsys.Remove("test_remove_symlink")
	if err != nil {
		t.Fatalf("Remove symlink failed: %v", err)
	}

	// Verify symlink is gone
	_, err = fsys.Lstat("test_remove_symlink")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Remove symlink effect not observed: symlink still exists or wrong error: %v", err)
	}

	// Clean up
	fsys.RemoveAll("test_remove_nonempty_dir")
}

// Test RemoveAll operations on complex directory structures
func testRemoveAll(t *testing.T, fsys vroot.Fs) {
	// Create a complex directory structure
	// test_removeall/
	//   ├── file1.txt
	//   ├── file2.txt
	//   ├── subdir1/
	//   │   ├── nested_file.txt
	//   │   └── deep/
	//   │       └── very_deep.txt
	//   ├── subdir2/
	//   │   └── another_file.txt
	//   └── symlink_to_file1

	basePath := "test_removeall"
	err := fsys.Mkdir(basePath, 0o755)
	if err != nil {
		t.Fatalf("Create base directory failed: %v", err)
	}

	// Create files in root
	files := []string{
		filepath.FromSlash("test_removeall/file1.txt"),
		filepath.FromSlash("test_removeall/file2.txt"),
	}
	for _, file := range files {
		f, err := fsys.Create(file)
		if err != nil {
			t.Fatalf("Create %s failed: %v", file, err)
		}
		f.Write([]byte("test content"))
		f.Close()
	}

	// Create subdirectories with files
	subdirs := []string{
		filepath.FromSlash("test_removeall/subdir1"),
		filepath.FromSlash("test_removeall/subdir1/deep"),
		filepath.FromSlash("test_removeall/subdir2"),
	}
	for _, dir := range subdirs {
		err = fsys.MkdirAll(dir, 0o755)
		if err != nil {
			t.Fatalf("MkdirAll %s failed: %v", dir, err)
		}
	}

	// Create nested files
	nestedFiles := []string{
		filepath.FromSlash("test_removeall/subdir1/nested_file.txt"),
		filepath.FromSlash("test_removeall/subdir1/deep/very_deep.txt"),
		filepath.FromSlash("test_removeall/subdir2/another_file.txt"),
	}
	for _, file := range nestedFiles {
		f, err := fsys.Create(file)
		if err != nil {
			t.Fatalf("Create %s failed: %v", file, err)
		}
		f.Write([]byte("nested content"))
		f.Close()
	}

	// Create a symlink
	err = fsys.Symlink("file1.txt", filepath.FromSlash("test_removeall/symlink_to_file1"))
	if err != nil {
		t.Fatalf("Create symlink failed: %v", err)
	}

	// Test RemoveAll on the entire structure
	err = fsys.RemoveAll(basePath)
	if err != nil {
		t.Fatalf("RemoveAll failed: %v", err)
	}

	// Verify everything is gone
	_, err = fsys.Stat(basePath)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("RemoveAll effect not observed: base directory still exists or wrong error: %v", err)
	}

	// Test RemoveAll on non-existent path (should succeed)
	err = fsys.RemoveAll("non_existent_path")
	if err != nil {
		t.Errorf("RemoveAll on non-existent path should succeed, got: %v", err)
	}

	// Test RemoveAll on a single file
	f, err := fsys.Create("single_file_removeall.txt")
	if err != nil {
		t.Fatalf("Create single file failed: %v", err)
	}
	f.Close()

	err = fsys.RemoveAll("single_file_removeall.txt")
	if err != nil {
		t.Errorf("RemoveAll on single file failed: %v", err)
	}

	_, err = fsys.Stat("single_file_removeall.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("RemoveAll on single file: file still exists or wrong error: %v", err)
	}

	// Test platform-specific RemoveAll with chmod (if applicable)
	testRemoveAllWithChmod(t, fsys)
}
