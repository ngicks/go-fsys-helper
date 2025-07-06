package osfslite

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestOsfsLite(t *testing.T) {
	tempDir := t.TempDir()
	osfsLite := New(tempDir)

	// Test file creation
	file, err := osfsLite.OpenFile("test.txt", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	file.Close()

	// Test stat
	info, err := osfsLite.Stat("test.txt")
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if info.Name() != "test.txt" {
		t.Errorf("unexpected file name: %s", info.Name())
	}

	// Test directory creation
	err = osfsLite.Mkdir("subdir", fs.ModePerm)
	if err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	// Test chmod
	err = osfsLite.Chmod("test.txt", 0o600)
	if err != nil {
		t.Fatalf("failed to chmod: %v", err)
	}

	// Verify base path is properly joined
	absPath := filepath.Join(tempDir, "test.txt")
	_, err = os.Stat(absPath)
	if err != nil {
		t.Errorf("file not created at expected path: %v", err)
	}
}

func TestFsWrapper(t *testing.T) {
	tempDir := t.TempDir()
	wrapper := NewFsWrapper(tempDir)

	// Test that wrapper implements fs.FS
	var _ fs.FS = wrapper

	// Create a test file first
	osfsLite := New(tempDir)
	file, err := osfsLite.OpenFile("test.txt", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	_, err = file.WriteString("test content")
	if err != nil {
		t.Fatalf("failed to write to file: %v", err)
	}
	file.Close()

	// Test fs.FS interface
	fsFile, err := wrapper.Open("test.txt")
	if err != nil {
		t.Fatalf("failed to open file via fs.FS: %v", err)
	}
	defer fsFile.Close()

	// Verify it returns fs.File
	var _ fs.File = fsFile

	// Test reading
	data := make([]byte, 12)
	n, err := fsFile.Read(data)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data[:n]) != "test content" {
		t.Errorf("unexpected file content: %s", string(data[:n]))
	}

	// Test that FsWrapper DOES implement ReadLinkFs interface (embedded from OsfsLite)
	type ReadLinkFs interface {
		ReadLink(name string) (string, error)
	}
	if _, hasReadLink := any(wrapper).(ReadLinkFs); !hasReadLink {
		t.Error("FsWrapper should implement ReadLinkFs interface")
	}
}

func TestBasicWrapper(t *testing.T) {
	tempDir := t.TempDir()
	wrapper := NewBasicWrapper(tempDir)

	// Test that wrapper implements fs.FS
	var _ fs.FS = wrapper

	// Create a test file first
	osfsLite := New(tempDir)
	file, err := osfsLite.OpenFile("test.txt", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	_, err = file.WriteString("test content")
	if err != nil {
		t.Fatalf("failed to write to file: %v", err)
	}
	file.Close()

	// Test fs.FS interface
	fsFile, err := wrapper.Open("test.txt")
	if err != nil {
		t.Fatalf("failed to open file via fs.FS: %v", err)
	}
	defer fsFile.Close()

	// Verify it returns fs.File
	var _ fs.File = fsFile

	// Test reading
	data := make([]byte, 12)
	n, err := fsFile.Read(data)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data[:n]) != "test content" {
		t.Errorf("unexpected file content: %s", string(data[:n]))
	}

	// Test Stat functionality
	info, err := wrapper.Stat("test.txt")
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if info.Name() != "test.txt" {
		t.Errorf("unexpected file name: %s", info.Name())
	}

	// Test that BasicWrapper does NOT implement ReadLinkFs interface
	type ReadLinkFs interface {
		ReadLink(name string) (string, error)
	}
	if _, hasReadLink := any(wrapper).(ReadLinkFs); hasReadLink {
		t.Error("BasicWrapper should not implement ReadLinkFs interface")
	}

	// Test that BasicWrapper does NOT implement other extended interfaces
	type MkdirFs interface {
		Mkdir(name string, perm fs.FileMode) error
	}
	if _, hasMkdir := any(wrapper).(MkdirFs); hasMkdir {
		t.Error("BasicWrapper should not implement MkdirFs interface")
	}
}