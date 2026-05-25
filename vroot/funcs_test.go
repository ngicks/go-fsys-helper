package vroot_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

// noExtFs exposes only the base Fs[*os.File] method set: because the embedded
// type is the interface (not the concrete *osfs.Fs), ReadDir/ReadFile are NOT
// part of its method set, so vroot.ReadDir / vroot.ReadFile cannot type-assert
// the extension interfaces and must use their Open-based fallback paths.
type noExtFs struct {
	vroot.Fs[*os.File]
}

// recordReadDirFs satisfies vroot.ReadDirFs[*os.File]; ReadDir records that the
// fast path was taken. The embedded Fs is unused on the fast path and may be nil.
type recordReadDirFs struct {
	vroot.Fs[*os.File]
	called *bool
}

func (f recordReadDirFs) ReadDir(string) ([]fs.DirEntry, error) {
	*f.called = true
	return nil, nil
}

// recordReadFileFs satisfies vroot.ReadFileFs[*os.File].
type recordReadFileFs struct {
	vroot.Fs[*os.File]
	called *bool
}

func (f recordReadFileFs) ReadFile(string) ([]byte, error) {
	*f.called = true
	return []byte("sentinel"), nil
}

// recordSubFs satisfies vroot.SubFs[*os.File].
type recordSubFs struct {
	vroot.Fs[*os.File]
	called *bool
}

func (f recordSubFs) Sub(string) (vroot.Fs[*os.File], error) {
	*f.called = true
	return f.Fs, nil
}

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

func TestWriteFile_OpenError(t *testing.T) {
	// OpenFile fails (path escapes the root), so WriteFile must return that
	// error rather than silently succeeding.
	tempDir := t.TempDir()
	fsys, err := osfs.NewFs(tempDir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	if err := vroot.WriteFile(fsys, "../escape.txt", []byte("x"), 0o644); err == nil {
		t.Error("WriteFile to escaping path: want error, got nil")
	}
}

// TestReadDir_FastPath verifies vroot.ReadDir delegates to ReadDirFs.ReadDir
// when the Fs implements the extension interface.
func TestReadDir_FastPath(t *testing.T) {
	called := false
	if _, err := vroot.ReadDir[*os.File](recordReadDirFs{called: &called}, "anything"); err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if !called {
		t.Error("ReadDir did not use the ReadDirFs fast path")
	}
}

// TestReadDir_Fallback verifies the Open-based fallback when the Fs does not
// implement ReadDirFs.
func TestReadDir_Fallback(t *testing.T) {
	tempDir := t.TempDir()
	writeFile(t, filepath.Join(tempDir, "a.txt"), "a")
	inner, err := osfs.NewFs(tempDir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	entries, err := vroot.ReadDir[*os.File](noExtFs{inner}, ".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "a.txt" {
		t.Errorf("fallback ReadDir = %v, want [a.txt]", entries)
	}
}

// TestReadFile_FastPath verifies vroot.ReadFile delegates to ReadFileFs.ReadFile.
func TestReadFile_FastPath(t *testing.T) {
	called := false
	got, err := vroot.ReadFile[*os.File](recordReadFileFs{called: &called}, "anything")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !called {
		t.Error("ReadFile did not use the ReadFileFs fast path")
	}
	if string(got) != "sentinel" {
		t.Errorf("ReadFile fast path = %q, want sentinel", got)
	}
}

// TestReadFile_Fallback verifies the Open+io.ReadAll fallback when the Fs does
// not implement ReadFileFs.
func TestReadFile_Fallback(t *testing.T) {
	tempDir := t.TempDir()
	writeFile(t, filepath.Join(tempDir, "f.txt"), "hello")
	inner, err := osfs.NewFs(tempDir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	got, err := vroot.ReadFile[*os.File](noExtFs{inner}, "f.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("fallback ReadFile = %q, want hello", got)
	}

	// Open failure in the fallback must propagate.
	if _, err := vroot.ReadFile[*os.File](noExtFs{inner}, "missing.txt"); err == nil {
		t.Error("fallback ReadFile on missing file: want error, got nil")
	}
}

// TestSub_FastPath verifies vroot.Sub delegates to SubFs.Sub when implemented.
func TestSub_FastPath(t *testing.T) {
	called := false
	if _, err := vroot.Sub[*os.File](recordSubFs{called: &called}, "dir"); err != nil {
		t.Fatalf("Sub: %v", err)
	}
	if !called {
		t.Error("Sub did not use the SubFs fast path")
	}
}

// TestSub_Fallback verifies vroot.Sub falls back to a PathPrefixFs view when
// the Fs has no native Sub, and that the result is scoped to dir.
func TestSub_Fallback(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tempDir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(tempDir, "sub", "f.txt"), "hi")

	fsys, err := osfs.NewFs(tempDir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	// osfs.Fs has OpenRoot, not Sub, so it does not satisfy SubFs -> fallback.
	sub, err := vroot.Sub[*os.File](fsys, "sub")
	if err != nil {
		t.Fatalf("Sub: %v", err)
	}
	got, err := vroot.ReadFile(sub, "f.txt")
	if err != nil {
		t.Fatalf("ReadFile in sub: %v", err)
	}
	if string(got) != "hi" {
		t.Errorf("sub-fs content = %q, want %q", got, "hi")
	}

	// Sub onto a non-existent dir is rejected by the PathPrefixFs validation.
	if _, err := vroot.Sub[*os.File](fsys, "missing"); err == nil {
		t.Error("Sub onto missing dir: want error, got nil")
	}
}
