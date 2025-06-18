package vroot_test

import (
	"errors"
	"fmt"
	"os"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

func Example_basic_usage() {
	tempDir, err := os.MkdirTemp("", "")
	if err != nil {
		panic(err)
	}

	root, err := osfs.NewRooted(tempDir)
	if err != nil {
		panic(err)
	}
	defer root.Close()

	// Safe file operations within the root
	file, err := root.Create("example.txt")
	if err != nil {
		_ = file.Close()
		panic(err)
	}
	_, err = file.Write([]byte("foofoobarbar"))
	if err != nil {
		_ = file.Close()
		panic(err)
	}
	_, err = file.WriteAt([]byte("baz"), 3)
	if err != nil {
		_ = file.Close()
		panic(err)
	}

	err = file.Sync()
	if err != nil {
		_ = file.Close()
		panic(err)
	}

	_ = file.Close()
	bin, err := root.ReadFile("example.txt")
	if err != nil {
		panic(err)
	}
	fmt.Printf("example.txt = %q\n", string(bin))
	// example.txt = "foobazbarbar"

	// path escaping from root fails (has ".." or absolute path after cleaned with filepath.Clean)
	_, err = root.Open("../../../etc/passwd")
	if !errors.Is(err, vroot.ErrPathEscapes) {
		panic(fmt.Errorf("should fail with %v", vroot.ErrPathEscapes))
	}
	fmt.Printf("path escapes: %v\n", err)
	// path escapes: openat ../../../etc/passwd: path escapes from parent

	// making link itself succeeds.
	err = root.Symlink("../../../etc/passwd", "link")
	if err != nil {
		panic(err)
	}

	// But opening it fails
	_, err = root.Open("link")
	if !errors.Is(err, vroot.ErrPathEscapes) {
		panic(fmt.Errorf("should fail with %v", vroot.ErrPathEscapes))
	}
	fmt.Printf("symlink escapes: %v\n", err)
	// symlink escapes: openat link: path escapes from parent

	// Output:
	// example.txt = "foobazbarbar"
	// path escapes: openat ../../../etc/passwd: path escapes from parent
	// symlink escapes: openat link: path escapes from parent
}
