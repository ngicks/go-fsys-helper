package overlay

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOverlay_EdgeCases(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)

	r, closers := prepareLayers(tempDir)
	defer r.Close()
	defer closers(t)

	t.Run("concurrent file operations", func(t *testing.T) {
		// Create file in top layer
		if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
			t.Fatal(err)
		}

		testFile, err := r.top.Create("root/writable/concurrent.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testFile.Write([]byte("initial")); err != nil {
			testFile.Close()
			t.Fatal(err)
		}
		testFile.Close()

		// Open multiple handles
		f1, err := r.OpenFile("root/writable/concurrent.txt", os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f1.Close()

		f2, err := r.OpenFile("root/writable/concurrent.txt", os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer f2.Close()

		// Concurrent writes
		go func() {
			_, _ = f1.Write([]byte(" from f1"))
		}()

		_, err = f2.Write([]byte(" from f2"))
		if err != nil {
			t.Errorf("concurrent write failed: %v", err)
		}

		time.Sleep(10 * time.Millisecond) // Let goroutine complete
	})

	t.Run("zero byte files", func(t *testing.T) {
		// Create empty file
		if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
			t.Fatal(err)
		}

		f, err := r.Create("root/writable/empty.txt")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()

		// Verify it exists and is empty
		info, err := r.Lstat("root/writable/empty.txt")
		if err != nil {
			t.Errorf("empty file should exist: %v", err)
		}
		if info.Size() != 0 {
			t.Errorf("expected size 0, got %d", info.Size())
		}

		// Read from empty file
		f, err = r.Open("root/writable/empty.txt")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		buf := make([]byte, 10)
		n, err := f.Read(buf)
		if err != io.EOF {
			t.Errorf("expected EOF from empty file, got %v", err)
		}
		if n != 0 {
			t.Errorf("expected 0 bytes read, got %d", n)
		}
	})

	t.Run("very long paths", func(t *testing.T) {
		// Create deeply nested directory structure
		longPath := "root/writable"
		for i := 0; i < 50; i++ {
			longPath = filepath.Join(longPath, "very_long_directory_name_that_creates_deep_nesting")
		}

		err := r.MkdirAll(longPath, fs.ModePerm)
		if err != nil {
			// Might fail due to path length limits
			t.Logf("long path creation failed (expected on some systems): %v", err)
			return
		}

		// Try to create file in deep directory
		filePath := filepath.Join(longPath, "deep_file.txt")
		f, err := r.Create(filePath)
		if err != nil {
			t.Logf("deep file creation failed: %v", err)
		} else {
			f.Close()
			t.Log("deep file creation succeeded")
		}
	})

	t.Run("special characters in filenames", func(t *testing.T) {
		if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
			t.Fatal(err)
		}

		specialNames := []string{
			"file with spaces.txt",
			"file-with-dashes.txt",
			"file_with_underscores.txt",
			"file.with.dots.txt",
			"file(with)parentheses.txt",
			"file[with]brackets.txt",
			"file{with}braces.txt",
		}

		for _, name := range specialNames {
			f, err := r.top.Create(filepath.Join("root/writable", name))
			if err != nil {
				t.Errorf("failed to create file with special name %q: %v", name, err)
				continue
			}
			f.Close()

			// Verify it can be accessed
			_, err = r.Lstat(filepath.Join("root/writable", name))
			if err != nil {
				t.Errorf("failed to stat file with special name %q: %v", name, err)
			}
		}
	})

	t.Run("file timestamps", func(t *testing.T) {
		if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
			t.Fatal(err)
		}

		f, err := r.top.Create("root/writable/timestamp_test.txt")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()

		// Set custom timestamp
		customTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
		err = r.Chtimes("root/writable/timestamp_test.txt", customTime, customTime)
		if err != nil {
			t.Errorf("failed to set timestamp: %v", err)
		}

		// Verify timestamp
		info, err := r.Lstat("root/writable/timestamp_test.txt")
		if err != nil {
			t.Fatal(err)
		}

		// Allow some tolerance for file system precision
		if abs(info.ModTime().Sub(customTime)) > time.Second {
			t.Errorf("unexpected modification time: got %v, expected ~%v", info.ModTime(), customTime)
		}
	})

	t.Run("symlink cycles", func(t *testing.T) {
		if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
			t.Fatal(err)
		}

		// Create symlink cycle: a -> b -> a
		err := r.Symlink("b", "root/writable/a")
		if err != nil {
			t.Fatal(err)
		}

		err = r.Symlink("a", "root/writable/b")
		if err != nil {
			t.Fatal(err)
		}

		// Trying to stat should detect cycle
		_, err = r.Stat("root/writable/a")
		if err == nil {
			t.Error("expected error when following symlink cycle")
		}
		if !strings.Contains(err.Error(), "cycle") && !strings.Contains(err.Error(), "loop") {
			t.Logf("symlink cycle error: %v", err)
		}
	})

	t.Run("directory traversal attempts", func(t *testing.T) {
		// These should be blocked by the rooted filesystem layer
		dangerousPaths := []string{
			"../../../etc/passwd",
			"root/../../etc/passwd",
			"root/writable/../../../etc/passwd",
		}

		for _, path := range dangerousPaths {
			_, err := r.Open(path)
			if err == nil {
				t.Errorf("dangerous path %q should be rejected", path)
			}

			_, err = r.Create(path)
			if err == nil {
				t.Errorf("dangerous path creation %q should be rejected", path)
			}
		}
	})

	t.Run("large file operations", func(t *testing.T) {
		if err := r.top.MkdirAll("root/writable", fs.ModePerm); err != nil {
			t.Fatal(err)
		}

		f, err := r.Create("root/writable/large.txt")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()

		// Write large amount of data (but not too large for tests)
		data := make([]byte, 1024*1024) // 1MB
		for i := range data {
			data[i] = byte(i % 256)
		}

		n, err := f.Write(data)
		if err != nil {
			t.Errorf("failed to write large data: %v", err)
		}
		if n != len(data) {
			t.Errorf("incomplete write: %d/%d bytes", n, len(data))
		}

		// Verify file size
		info, err := r.Lstat("root/writable/large.txt")
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() != int64(len(data)) {
			t.Errorf("unexpected file size: %d", info.Size())
		}
	})
}

func TestOverlay_ErrorHandling(t *testing.T) {
	tempDir := t.TempDir()
	t.Logf("temp dir = %s", tempDir)

	r, closers := prepareLayers(tempDir)
	defer r.Close()
	defer closers(t)

	t.Run("operations on non-existent files", func(t *testing.T) {
		ops := []struct {
			name string
			op   func() error
		}{
			{"Open", func() error { _, err := r.Open("nonexistent.txt"); return err }},
			{"Remove", func() error { return r.Remove("nonexistent.txt") }},
			{"Chmod", func() error { return r.Chmod("nonexistent.txt", 0o644) }},
			{"Chtimes", func() error { return r.Chtimes("nonexistent.txt", time.Now(), time.Now()) }},
			{"ReadLink", func() error { _, err := r.ReadLink("nonexistent.txt"); return err }},
		}

		for _, op := range ops {
			err := op.op()
			if err == nil {
				t.Errorf("%s on non-existent file should fail", op.name)
			}
			if !errors.Is(err, fs.ErrNotExist) {
				t.Logf("%s error: %v", op.name, err)
			}
		}
	})

	t.Run("operations on wrong file types", func(t *testing.T) {
		// Create directory
		if err := r.top.MkdirAll("root/writable/testdir", fs.ModePerm); err != nil {
			t.Fatal(err)
		}

		// Try to read directory as file
		_, err := r.Open("root/writable/testdir")
		// This might succeed (directories can be opened) or fail depending on implementation
		t.Logf("opening directory as file: %v", err)

		// Create file
		f, err := r.top.Create("root/writable/testfile.txt")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()

		// Try directory operations on file
		err = r.Mkdir("root/writable/testfile.txt/subdir", fs.ModePerm)
		if err == nil {
			t.Error("mkdir on file should fail")
		}
	})
}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
