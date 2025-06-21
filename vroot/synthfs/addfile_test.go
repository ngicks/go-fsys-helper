package synthfs

import (
	"embed"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/clock"
)

//go:embed testdata
var testdata embed.FS

func TestAddFile(t *testing.T) {
	// Create a synthetic filesystem
	allocator := NewMemFileAllocator(clock.RealWallClock())
	synth := NewRooted("test://", allocator, Option{
		Clock: clock.RealWallClock(),
	})

	// Test adding a file to root directory
	t.Run("add file to root", func(t *testing.T) {
		// Create a file view from embedded FS
		view, err := NewFsFileView(testdata, "testdata/hello.txt")
		if err != nil {
			t.Fatalf("NewFsFileView failed: %v", err)
		}

		// Add the file
		err = synth.AddFile("hello.txt", view, 0o755, 0o644)
		if err != nil {
			t.Fatalf("AddFile failed: %v", err)
		}

		// Verify the file exists and has correct content
		content, err := vroot.ReadFile(synth, "hello.txt")
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}

		expected := "Hello, World!"
		if string(content) != expected {
			t.Errorf("File content mismatch: got %q, want %q", string(content), expected)
		}

		// Verify file permissions
		info, err := synth.Stat("hello.txt")
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}

		// With default umask 0o022, 0o644 should become 0o644
		if info.Mode().Perm() != 0o644 {
			t.Errorf("File permissions mismatch: got %o, want %o", info.Mode().Perm(), 0o644)
		}
	})

	// Test adding a file with parent directory creation
	t.Run("add file with parent creation", func(t *testing.T) {
		view, err := NewFsFileView(testdata, "testdata/hello.txt")
		if err != nil {
			t.Fatalf("NewFsFileView failed: %v", err)
		}

		// Add file to non-existent directory
		err = synth.AddFile(filepath.FromSlash("newdir/subdir/test.txt"), view, 0o755, 0o666)
		if err != nil {
			t.Fatalf("AddFile failed: %v", err)
		}

		// Verify directories were created
		info, err := synth.Stat("newdir")
		if err != nil {
			t.Fatalf("Stat newdir failed: %v", err)
		}
		if !info.IsDir() {
			t.Error("newdir should be a directory")
		}

		info, err = synth.Stat(filepath.FromSlash("newdir/subdir"))
		if err != nil {
			t.Fatalf("Stat newdir/subdir failed: %v", err)
		}
		if !info.IsDir() {
			t.Error("newdir/subdir should be a directory")
		}

		// Verify file exists
		content, err := vroot.ReadFile(synth, filepath.FromSlash("newdir/subdir/test.txt"))
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(content) != "Hello, World!" {
			t.Errorf("File content mismatch")
		}
	})

	// Test error cases
	t.Run("add file errors", func(t *testing.T) {
		view, err := NewFsFileView(testdata, "testdata/hello.txt")
		if err != nil {
			t.Fatalf("NewFsFileView failed: %v", err)
		}

		// Try to add file that already exists
		err = synth.AddFile("hello.txt", view, 0o755, 0o644)
		if !errors.Is(err, fs.ErrExist) {
			t.Errorf("Expected ErrExist, got: %v", err)
		}
	})
}

func TestAddFs(t *testing.T) {
	// Create a synthetic filesystem
	allocator := NewMemFileAllocator(clock.RealWallClock())
	synth := NewRooted("test://", allocator, Option{
		Clock: clock.RealWallClock(),
	})

	// Create a test filesystem with real directory
	tempDir := t.TempDir()
	// Create test structure
	err := os.MkdirAll(filepath.Join(tempDir, "testdata", "subdir"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}
	err = os.WriteFile(filepath.Join(tempDir, "testdata", "hello.txt"), []byte("Hello, World!"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	err = os.WriteFile(filepath.Join(tempDir, "testdata", "subdir", "nested.txt"), []byte("Nested file"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create nested test file: %v", err)
	}

	// Convert to vroot.Fs using os.DirFS
	testdataVroot := vroot.FromIoFsRooted(os.DirFS(tempDir).(fs.ReadLinkFS), "testdata://")

	// Test adding entire filesystem to root
	t.Run("add fs to root", func(t *testing.T) {
		err := synth.AddFs(".", testdataVroot, 0o755)
		if err != nil {
			t.Fatalf("AddFs failed: %v", err)
		}

		// Verify all files were added
		entries, err := vroot.ReadDir(synth, "testdata")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}

		if len(entries) < 2 {
			t.Errorf("Expected at least 2 entries in testdata, got %d", len(entries))
		}

		// Verify file content
		content, err := vroot.ReadFile(synth, filepath.FromSlash("testdata/hello.txt"))
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(content) != "Hello, World!" {
			t.Errorf("File content mismatch")
		}

		// Verify nested directory
		content, err = vroot.ReadFile(synth, filepath.FromSlash("testdata/subdir/nested.txt"))
		if err != nil {
			t.Fatalf("ReadFile testdata/subdir/nested.txt failed: %v", err)
		}
		if string(content) != "Nested file" {
			t.Errorf("Nested file content mismatch")
		}
	})

	// Test adding filesystem to subdirectory
	t.Run("add fs to subdirectory", func(t *testing.T) {
		synth2 := NewRooted("test2://", allocator, Option{
			Clock: clock.RealWallClock(),
		})

		err := synth2.AddFs("imported", testdataVroot, 0o755)
		if err != nil {
			t.Fatalf("AddFs failed: %v", err)
		}

		// Verify root directory was created
		info, err := synth2.Stat("imported")
		if err != nil {
			t.Fatalf("Stat imported failed: %v", err)
		}
		if !info.IsDir() {
			t.Error("imported should be a directory")
		}

		// Verify files are under the root
		content, err := vroot.ReadFile(synth2, filepath.FromSlash("imported/testdata/hello.txt"))
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(content) != "Hello, World!" {
			t.Errorf("File content mismatch")
		}
	})
}

func TestAddFileWithRangedView(t *testing.T) {
	// Create a synthetic filesystem
	allocator := NewMemFileAllocator(clock.RealWallClock())
	synth := NewRooted("test://", allocator, Option{
		Clock: clock.RealWallClock(),
	})

	// Create a ranged view of hello.txt (first 5 bytes)
	view, err := NewRangedFsFileView(testdata, "testdata/hello.txt", 0, 5)
	if err != nil {
		t.Fatalf("NewRangedFsFileView failed: %v", err)
	}

	// Add the ranged file
	err = synth.AddFile("partial.txt", view, 0o755, 0o644)
	if err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	// Read and verify
	f, err := synth.Open("partial.txt")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	expected := "Hello"
	if string(content) != expected {
		t.Errorf("Content mismatch: got %q, want %q", string(content), expected)
	}

	// Verify size
	info, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size() != 5 {
		t.Errorf("Size mismatch: got %d, want 5", info.Size())
	}
}

func TestAddFileIntegration(t *testing.T) {
	// Test that added files integrate well with the rest of the filesystem
	allocator := NewMemFileAllocator(clock.RealWallClock())
	synth := NewRooted("test://", allocator, Option{
		Clock: clock.RealWallClock(),
	})

	// Create some files using the allocator
	err := vroot.WriteFile(synth, "dynamic1.txt", []byte("Dynamic content 1"), 0o644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Add files from embedded FS
	view, err := NewFsFileView(testdata, "testdata/hello.txt")
	if err != nil {
		t.Fatalf("NewFsFileView failed: %v", err)
	}
	err = synth.AddFile("static1.txt", view, 0o755, 0o644)
	if err != nil {
		t.Fatalf("AddFile failed: %v", err)
	}

	// Create more dynamic files
	err = vroot.WriteFile(synth, "dynamic2.txt", []byte("Dynamic content 2"), 0o644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// List all files
	entries, err := vroot.ReadDir(synth, ".")
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	expectedFiles := map[string]bool{
		"dynamic1.txt": false,
		"static1.txt":  false,
		"dynamic2.txt": false,
	}

	for _, entry := range entries {
		if _, ok := expectedFiles[entry.Name()]; ok {
			expectedFiles[entry.Name()] = true
		}
	}

	for name, found := range expectedFiles {
		if !found {
			t.Errorf("Expected file %q not found", name)
		}
	}

	// Verify we can read all files
	content1, _ := vroot.ReadFile(synth, "dynamic1.txt")
	if string(content1) != "Dynamic content 1" {
		t.Error("dynamic1.txt content mismatch")
	}

	content2, _ := vroot.ReadFile(synth, "static1.txt")
	if string(content2) != "Hello, World!" {
		t.Error("static1.txt content mismatch")
	}

	content3, _ := vroot.ReadFile(synth, "dynamic2.txt")
	if string(content3) != "Dynamic content 2" {
		t.Error("dynamic2.txt content mismatch")
	}
}

// Create an in-memory test filesystem for AddFs tests
func createTestFS() fs.FS {
	return fstest.MapFS{
		"file1.txt": &fstest.MapFile{
			Data: []byte("File 1 content"),
		},
		"dir1/file2.txt": &fstest.MapFile{
			Data: []byte("File 2 content"),
		},
		"dir1/dir2/file3.txt": &fstest.MapFile{
			Data: []byte("File 3 content"),
		},
	}
}

func TestAddFsWithMapFS(t *testing.T) {
	allocator := NewMemFileAllocator(clock.RealWallClock())
	synth := NewRooted("test://", allocator, Option{
		Clock: clock.RealWallClock(),
	})

	// Create a temporary directory with test structure
	tempDir := t.TempDir()
	err := os.MkdirAll(filepath.Join(tempDir, "dir1", "dir2"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create test dirs: %v", err)
	}
	err = os.WriteFile(filepath.Join(tempDir, "file1.txt"), []byte("File 1 content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create file1.txt: %v", err)
	}
	err = os.WriteFile(filepath.Join(tempDir, "dir1", "file2.txt"), []byte("File 2 content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create file2.txt: %v", err)
	}
	err = os.WriteFile(filepath.Join(tempDir, "dir1", "dir2", "file3.txt"), []byte("File 3 content"), 0o644)
	if err != nil {
		t.Fatalf("Failed to create file3.txt: %v", err)
	}

	testFSVroot := vroot.FromIoFsRooted(os.DirFS(tempDir).(fs.ReadLinkFS), "testfs://")

	err = synth.AddFs("mapped", testFSVroot, 0o755)
	if err != nil {
		t.Fatalf("AddFs failed: %v", err)
	}

	// Verify structure
	content, err := vroot.ReadFile(synth, filepath.FromSlash("mapped/file1.txt"))
	if err != nil {
		t.Fatalf("ReadFile mapped/file1.txt failed: %v", err)
	}
	if string(content) != "File 1 content" {
		t.Error("file1.txt content mismatch")
	}

	content, err = vroot.ReadFile(synth, filepath.FromSlash("mapped/dir1/dir2/file3.txt"))
	if err != nil {
		t.Fatalf("ReadFile mapped/dir1/dir2/file3.txt failed: %v", err)
	}
	if string(content) != "File 3 content" {
		t.Error("file3.txt content mismatch")
	}
}
