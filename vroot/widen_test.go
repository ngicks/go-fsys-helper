package vroot_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func TestWiden_osfs(t *testing.T) {
	tmp := t.TempDir()
	inner, err := osfs.NewFs(tmp)
	if err != nil {
		t.Fatalf("osfs.NewFs: %v", err)
	}
	defer inner.Close()

	g := vroot.Widen(inner)

	// Write through the widened Fs.
	f, err := g.Create("hello.txt")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := f.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read back through Open.
	rf, err := g.Open("hello.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	data, err := io.ReadAll(rf)
	_ = rf.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("content = %q, want %q", data, "hello")
	}

	// The file really exists on the OS filesystem under tmp.
	if _, err := os.Stat(filepath.Join(tmp, "hello.txt")); err != nil {
		t.Errorf("file not on disk: %v", err)
	}

	// ReadFile extension forwards to osfs's native ReadFile.
	if rb, err := vroot.ReadFile(g, "hello.txt"); err != nil || string(rb) != "hello" {
		t.Errorf("ReadFile = %q, %v; want %q, nil", rb, err, "hello")
	}

	// ReadDir extension.
	ents, err := vroot.ReadDir(g, ".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(ents) != 1 || ents[0].Name() != "hello.txt" {
		t.Errorf("ReadDir = %v, want [hello.txt]", ents)
	}

	// Sub returns a widened sub-fs.
	if err := g.Mkdir("sub", 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	sub, err := vroot.Sub(g, "sub")
	if err != nil {
		t.Fatalf("Sub: %v", err)
	}
	if err := vroot.WriteFile(sub, "inner.txt", []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile in sub: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "sub", "inner.txt")); err != nil {
		t.Errorf("sub file not on disk: %v", err)
	}
}

func TestWiden_alreadyWide(t *testing.T) {
	tmp := t.TempDir()
	inner, err := osfs.NewFs(tmp)
	if err != nil {
		t.Fatalf("osfs.NewFs: %v", err)
	}
	defer inner.Close()

	g := vroot.Widen(inner)
	// Widening an Fs[File] must be a no-op returning the same value.
	g2 := vroot.Widen(g)
	if g2 != g {
		t.Errorf("Widen(Fs[File]) returned a new wrapper, want identical value")
	}
}
