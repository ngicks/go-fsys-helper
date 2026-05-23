package vroot_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("setup: write %s: %v", path, err)
	}
}

func TestReadDir(t *testing.T) {
	tempDir := t.TempDir()
	writeFile(t, filepath.Join(tempDir, "c.txt"), "c")
	writeFile(t, filepath.Join(tempDir, "a.txt"), "a")
	writeFile(t, filepath.Join(tempDir, "b.txt"), "b")

	fsys, err := osfs.NewFs(tempDir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}

	entries, err := vroot.ReadDir(fsys, ".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{"a.txt", "b.txt", "c.txt"}
	if !slices.Equal(names, want) {
		t.Errorf("ReadDir got %v, want %v (must be sorted)", names, want)
	}
}

func TestReadDir_NonExistent(t *testing.T) {
	tempDir := t.TempDir()
	fsys, err := osfs.NewFs(tempDir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}

	_, err = vroot.ReadDir(fsys, "missing")
	if err == nil {
		t.Fatal("ReadDir on missing dir: want error, got nil")
	}
}

func TestReadFile(t *testing.T) {
	tempDir := t.TempDir()
	writeFile(t, filepath.Join(tempDir, "f.txt"), "hello world")

	fsys, err := osfs.NewFs(tempDir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}

	data, err := vroot.ReadFile(fsys, "f.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("ReadFile = %q, want %q", data, "hello world")
	}
}

func TestReadFile_NonExistent(t *testing.T) {
	tempDir := t.TempDir()
	fsys, err := osfs.NewFs(tempDir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}

	_, err = vroot.ReadFile(fsys, "nope.txt")
	if err == nil {
		t.Fatal("ReadFile on missing file: want error, got nil")
	}
}

func TestFd(t *testing.T) {
	tempDir := t.TempDir()
	writeFile(t, filepath.Join(tempDir, "f.txt"), "x")

	fsys, err := osfs.NewFs(tempDir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	f, err := fsys.Open("f.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = f.Close() }()

	// osfs returns *os.File which has a real Fd.
	got := vroot.Fd(f)
	if got == ^uintptr(0) {
		t.Error("Fd returned invalid sentinel for *os.File; expected a real descriptor")
	}

	// A value not implementing Fd() returns the sentinel.
	if got := vroot.Fd("not a file"); got != ^uintptr(0) {
		t.Errorf("Fd(string) = %d, want sentinel ^(uintptr(0)) = %d", got, ^uintptr(0))
	}
}

func TestWriteFile(t *testing.T) {
	tempDir := t.TempDir()
	fsys, err := osfs.NewFs(tempDir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}

	if err := vroot.WriteFile(fsys, "out.txt", []byte("payload"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(tempDir, "out.txt"))
	if err != nil {
		t.Fatalf("ReadFile (check): %v", err)
	}
	if string(got) != "payload" {
		t.Errorf("file content = %q, want %q", got, "payload")
	}
}

func TestWriteFile_Truncates(t *testing.T) {
	// WriteFile uses O_TRUNC; a second write must replace, not append.
	tempDir := t.TempDir()
	fsys, err := osfs.NewFs(tempDir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}

	if err := vroot.WriteFile(fsys, "out.txt", []byte("longlonglong"), 0o644); err != nil {
		t.Fatalf("WriteFile (initial): %v", err)
	}
	if err := vroot.WriteFile(fsys, "out.txt", []byte("short"), 0o644); err != nil {
		t.Fatalf("WriteFile (overwrite): %v", err)
	}

	got, err := os.ReadFile(filepath.Join(tempDir, "out.txt"))
	if err != nil {
		t.Fatalf("ReadFile (check): %v", err)
	}
	if string(got) != "short" {
		t.Errorf("after truncating overwrite, content = %q, want %q", got, "short")
	}
}
