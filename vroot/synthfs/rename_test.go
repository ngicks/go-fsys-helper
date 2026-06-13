package synthfs_test

import (
	"errors"
	"io/fs"
	"syscall"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot/synthfs"
)

// TestRenameDirIntoOwnSubtree covers plan 01 V2: moving a directory into its own
// subtree must be rejected with EINVAL rather than re-parented into a detached
// cycle. The cycle bug manifested as a ".." walk that never reaches the
// boundary, so this test must complete well within `go test -timeout`.
func TestRenameDirIntoOwnSubtree(t *testing.T) {
	r := synthfs.NewRoot("synth://", nil)
	c := testhelper.New(t, r)
	c.SetupLines(
		"a/",
		"a/b/",
		`a/b/keep.txt: "k"`,
	)

	cases := []struct {
		name        string
		old, newPth string
	}{
		{"deep descendant", "a", "a/b/c"},
		{"direct child", "a", "a/c"},
		{"into self as parent", "a", "a/b"}, // a/b already exists (empty? no, has keep.txt)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := r.Rename(tc.old, tc.newPth)
			if !errors.Is(err, syscall.EINVAL) {
				t.Fatalf("Rename(%q,%q): want EINVAL, got %v", tc.old, tc.newPth, err)
			}
		})
	}

	// The tree must remain intact and ".."-walkable after every rejection: a
	// stat that walks back up through the boundary must terminate.
	if _, err := r.Stat("a/b/keep.txt"); err != nil {
		t.Fatalf("Stat(a/b/keep.txt) after rejected renames: %v", err)
	}
	if _, err := r.Stat("a/b/../b/keep.txt"); err != nil {
		t.Fatalf("Stat through ..-walk after rejected renames: %v", err)
	}
}

// TestRenameOntoSelfNoop covers the source==destination no-op path: it must
// succeed and leave the entry (and its contents) in place.
func TestRenameOntoSelfNoop(t *testing.T) {
	r := synthfs.NewRoot("synth://", nil)
	c := testhelper.New(t, r)
	c.SetupLines(
		"d/",
		`d/f.txt: "x"`,
	)
	if err := r.Rename("d", "d"); err != nil {
		t.Fatalf("Rename(d,d): want nil, got %v", err)
	}
	if err := r.Rename("d/f.txt", "d/f.txt"); err != nil {
		t.Fatalf("Rename(d/f.txt, d/f.txt): want nil, got %v", err)
	}
	if _, err := r.Stat("d/f.txt"); err != nil {
		t.Fatalf("Stat(d/f.txt) after self-rename: %v", err)
	}
}

// TestRenameDirMoveStillWorks guards against the subtree check over-firing: a
// legitimate directory move to a SIBLING subtree must still succeed.
func TestRenameDirMoveStillWorks(t *testing.T) {
	r := synthfs.NewRoot("synth://", nil)
	c := testhelper.New(t, r)
	c.SetupLines(
		"src/",
		"src/inner/",
		`src/inner/f.txt: "x"`,
		"dst/",
	)
	if err := r.Rename("src/inner", "dst/inner"); err != nil {
		t.Fatalf("Rename(src/inner, dst/inner): %v", err)
	}
	if _, err := r.Stat("dst/inner/f.txt"); err != nil {
		t.Fatalf("Stat(dst/inner/f.txt) after move: %v", err)
	}
	if _, err := r.Stat("src/inner"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("src/inner should be gone after move, got %v", err)
	}
}
