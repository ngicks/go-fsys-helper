package vroot_test

import (
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

// lstatFailFs wraps an *osfs.Fs and injects an Lstat error for one designated
// (cleaned, slash-form) child path, so a walk encounters an unreadable sibling
// while its neighbors remain readable.
type lstatFailFs struct {
	*osfs.Fs
	failOn string // slash-form path whose Lstat returns an error
}

func (f *lstatFailFs) Lstat(name string) (fs.FileInfo, error) {
	if filepath.ToSlash(filepath.Clean(name)) == f.failOn {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: fs.ErrPermission}
	}
	return f.Fs.Lstat(name)
}

// TestWalk_SiblingContinuesAfterLstatError covers plan 01 V4: a single
// unreadable child (its Lstat errors) must NOT truncate the directory — the walk
// must continue over the remaining siblings, mirroring filepath.WalkDir.
func TestWalk_SiblingContinuesAfterLstatError(t *testing.T) {
	dir := t.TempDir()
	// Five sibling files; "c.txt" will have a failing Lstat. Names chosen so the
	// failing one is in the middle of the sorted listing, proving entries after
	// it are still visited.
	setupLines(t, dir,
		`a.txt: "a"`,
		`b.txt: "b"`,
		`c.txt: "c"`,
		`d.txt: "d"`,
		`e.txt: "e"`,
	)
	base, err := osfs.NewFs(dir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	t.Cleanup(func() { _ = base.Close() })
	fsys := &lstatFailFs{Fs: base, failOn: "c.txt"}

	var visited []string
	var sawErrFor string
	err = vroot.WalkDir(fsys, ".", nil, func(path, realPath string, d fs.FileInfo, walkErr error) error {
		if walkErr != nil {
			sawErrFor = filepath.ToSlash(path)
			return nil // swallow the error; the walk must continue
		}
		visited = append(visited, filepath.ToSlash(path))
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir returned error: %v", err)
	}

	if sawErrFor != "c.txt" {
		t.Errorf("expected the Lstat error to be reported for c.txt, got %q", sawErrFor)
	}
	// Every readable sibling (and the root ".") must have been visited; c.txt is
	// excluded because its only callback delivered the error.
	want := []string{".", "a.txt", "b.txt", "d.txt", "e.txt"}
	slices.Sort(visited)
	if !slices.Equal(visited, want) {
		t.Errorf("siblings truncated after Lstat error:\n got  %v\n want %v", visited, want)
	}
}

// TestWalk_SiblingSkipAllAfterLstatError confirms SkipAll from the error
// callback still terminates the whole walk.
func TestWalk_SiblingSkipAllAfterLstatError(t *testing.T) {
	dir := t.TempDir()
	setupLines(t, dir,
		`a.txt: "a"`,
		`b.txt: "b"`,
		`c.txt: "c"`,
		`d.txt: "d"`,
	)
	base, err := osfs.NewFs(dir)
	if err != nil {
		t.Fatalf("NewFs: %v", err)
	}
	t.Cleanup(func() { _ = base.Close() })
	fsys := &lstatFailFs{Fs: base, failOn: "b.txt"}

	var visitedAfter int
	err = vroot.WalkDir(fsys, ".", nil, func(path, realPath string, d fs.FileInfo, walkErr error) error {
		if walkErr != nil {
			return vroot.SkipAll
		}
		if filepath.ToSlash(path) == "c.txt" || filepath.ToSlash(path) == "d.txt" {
			visitedAfter++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir with SkipAll: %v", err)
	}
	if visitedAfter != 0 {
		t.Errorf("SkipAll from the error callback should stop the walk, but %d later siblings were visited", visitedAfter)
	}
}
