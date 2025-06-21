package overlayfs

import (
	"errors"
	"os"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func TestLayerReturnsErrWhitedOut(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create a test file
	testFile := "test.txt"
	if err := os.WriteFile(tempDir+"/"+testFile, []byte("test content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create underlying filesystem
	fsys, err := osfs.NewRooted(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer fsys.Close()

	// Create a simple metadata store that marks our test file as whited out
	meta := &simpleTestMetadata{
		whitedOut: map[string]bool{
			testFile: true,
		},
	}

	// Create layer
	layer := NewLayer(meta, fsys)

	// Test that Stat returns ErrWhitedOut
	_, err = layer.Stat(testFile)
	if !errors.Is(err, ErrWhitedOut) {
		t.Errorf("Expected ErrWhitedOut, got %v", err)
	}

	// Test that Open returns ErrWhitedOut
	_, err = layer.Open(testFile)
	if !errors.Is(err, ErrWhitedOut) {
		t.Errorf("Expected ErrWhitedOut, got %v", err)
	}

	// Test that Lstat returns ErrWhitedOut
	_, err = layer.Lstat(testFile)
	if !errors.Is(err, ErrWhitedOut) {
		t.Errorf("Expected ErrWhitedOut, got %v", err)
	}
}

// Simple test metadata store implementation
type simpleTestMetadata struct {
	whitedOut map[string]bool
}

func (m *simpleTestMetadata) QueryWhiteout(path string) (bool, error) {
	return m.whitedOut[path], nil
}

func (m *simpleTestMetadata) RecordWhiteout(path string) error {
	m.whitedOut[path] = true
	return nil
}

func (m *simpleTestMetadata) RemoveWhiteout(path string) error {
	delete(m.whitedOut, path)
	return nil
}

func (m *simpleTestMetadata) Close() error {
	return nil
}
