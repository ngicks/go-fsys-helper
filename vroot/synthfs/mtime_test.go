package synthfs

import (
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/clock"
)

func TestMtimeUpdateOnWrite(t *testing.T) {
	// Create a synthetic filesystem
	allocator := NewMemFileAllocator(clock.RealWallClock())
	synth := NewRooted("test://", allocator, Option{
		Clock: clock.RealWallClock(),
	})

	// Create a test file
	file, err := synth.Create("test.txt")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get initial mtime
	info1, err := file.Stat()
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	initialMtime := info1.ModTime()

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Write to the file
	_, err = file.Write([]byte("Hello, World!"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Check that mtime was updated via file handle
	info2, err := file.Stat()
	if err != nil {
		t.Fatalf("Stat after write failed: %v", err)
	}
	fileMtime := info2.ModTime()

	// Check that mtime was updated via filesystem
	info3, err := synth.Stat("test.txt")
	if err != nil {
		t.Fatalf("Filesystem Stat failed: %v", err)
	}
	fsMtime := info3.ModTime()

	file.Close()

	// Verify that mtime was updated
	if !fileMtime.After(initialMtime) {
		t.Errorf("File mtime should be updated after write: initial=%v, after=%v", initialMtime, fileMtime)
	}

	if !fsMtime.After(initialMtime) {
		t.Errorf("Filesystem mtime should be updated after write: initial=%v, after=%v", initialMtime, fsMtime)
	}

	// The file handle mtime and filesystem mtime should be the same (or very close)
	if fsMtime.Sub(fileMtime).Abs() > time.Millisecond {
		t.Errorf("File and filesystem mtime should be synchronized: file=%v, fs=%v", fileMtime, fsMtime)
	}
}

func TestMtimeUpdateOnTruncate(t *testing.T) {
	// Create a synthetic filesystem
	allocator := NewMemFileAllocator(clock.RealWallClock())
	synth := NewRooted("test://", allocator, Option{
		Clock: clock.RealWallClock(),
	})

	// Create a test file with content
	err := vroot.WriteFile(synth, "test.txt", []byte("Initial content"), 0o644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Get initial mtime
	info1, err := synth.Stat("test.txt")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	initialMtime := info1.ModTime()

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Open file and truncate it
	file, err := synth.OpenFile("test.txt", 0o2, 0o644) // O_RDWR
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer file.Close()

	err = file.Truncate(5)
	if err != nil {
		t.Fatalf("Truncate failed: %v", err)
	}

	// Check that mtime was updated via filesystem
	info2, err := synth.Stat("test.txt")
	if err != nil {
		t.Fatalf("Stat after truncate failed: %v", err)
	}
	afterMtime := info2.ModTime()

	// Verify that mtime was updated
	if !afterMtime.After(initialMtime) {
		t.Errorf("Mtime should be updated after truncate: initial=%v, after=%v", initialMtime, afterMtime)
	}
}

func TestMtimeUpdateOnWriteAt(t *testing.T) {
	// Create a synthetic filesystem
	allocator := NewMemFileAllocator(clock.RealWallClock())
	synth := NewRooted("test://", allocator, Option{
		Clock: clock.RealWallClock(),
	})

	// Create a test file with initial content
	err := vroot.WriteFile(synth, "test.txt", []byte("Hello, World!"), 0o644)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Get initial mtime
	info1, err := synth.Stat("test.txt")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	initialMtime := info1.ModTime()

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Open file and write at offset
	file, err := synth.OpenFile("test.txt", 0o2, 0o644) // O_RDWR
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	defer file.Close()

	_, err = file.WriteAt([]byte("TEST"), 0)
	if err != nil {
		t.Fatalf("WriteAt failed: %v", err)
	}

	// Check that mtime was updated via filesystem
	info2, err := synth.Stat("test.txt")
	if err != nil {
		t.Fatalf("Stat after WriteAt failed: %v", err)
	}
	afterMtime := info2.ModTime()

	// Verify that mtime was updated
	if !afterMtime.After(initialMtime) {
		t.Errorf("Mtime should be updated after WriteAt: initial=%v, after=%v", initialMtime, afterMtime)
	}

	// Verify content was updated
	content, err := vroot.ReadFile(synth, "test.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	expected := "TESTo, World!" // "Hello" -> "TESTo" (WriteAt replaces 4 bytes starting at offset 0)
	if string(content) != expected {
		t.Errorf("Content mismatch: got %q, want %q", string(content), expected)
	}
}

func TestMtimeUpdateOnWriteString(t *testing.T) {
	// Create a synthetic filesystem
	allocator := NewMemFileAllocator(clock.RealWallClock())
	synth := NewRooted("test://", allocator, Option{
		Clock: clock.RealWallClock(),
	})

	// Create a test file
	file, err := synth.Create("test.txt")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer file.Close()

	// Get initial mtime
	info1, err := synth.Stat("test.txt")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	initialMtime := info1.ModTime()

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Write string to the file
	_, err = file.WriteString("Hello from WriteString!")
	if err != nil {
		t.Fatalf("WriteString failed: %v", err)
	}

	// Check that mtime was updated via filesystem
	info2, err := synth.Stat("test.txt")
	if err != nil {
		t.Fatalf("Stat after WriteString failed: %v", err)
	}
	afterMtime := info2.ModTime()

	// Verify that mtime was updated
	if !afterMtime.After(initialMtime) {
		t.Errorf("Mtime should be updated after WriteString: initial=%v, after=%v", initialMtime, afterMtime)
	}
}
