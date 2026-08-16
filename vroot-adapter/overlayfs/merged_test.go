package overlayfs

import (
	"errors"
	"io/fs"
	"syscall"
	"testing"
)

// TestOverlayStateLookupOrder checks mount order: the top shadows every lower,
// lowers[0] shadows lowers[1], and a name only the deepest layer has is still
// found.
func TestOverlayStateLookupOrder(t *testing.T) {
	top, lower0, lower1 := threeLayers(t)
	seed(t, top, "everywhere", "top-only")
	seed(t, lower0, "everywhere", "both-lowers", "lower0-only")
	seed(t, lower1, "everywhere", "both-lowers", "lower1-only")

	s := newState(t, top, lower0, lower1)

	assertLayer(t, s, "everywhere", 0)
	assertLayer(t, s, "top-only", 0)
	assertLayer(t, s, "both-lowers", 1)
	assertLayer(t, s, "lower0-only", 1)
	assertLayer(t, s, "lower1-only", 2)
	assertLookupErr(t, s, "nowhere", fs.ErrNotExist)

	s.mu.RLock()
	defer s.mu.RUnlock()
	res, err := s.lookupFrom("everywhere", 1)
	if err != nil {
		t.Fatalf("lookupFrom(everywhere, 1): %v", err)
	}
	if res.layer != 1 {
		t.Errorf("lookupFrom(everywhere, 1) -> layer %d, want 1", res.layer)
	}
	if _, err := s.lookupFrom("top-only", 1); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("lookupFrom(top-only, 1) -> %v, want an error matching fs.ErrNotExist", err)
	}
}

// TestOverlayStateReadDirMerge checks the merged listing: sorted, one entry per
// name, and each entry the one the shallowest layer holding it has.
func TestOverlayStateReadDirMerge(t *testing.T) {
	top, lower0, lower1 := threeLayers(t)
	seed(t, top, "b/", "z")
	seed(t, lower0, "a", "b", "y/")
	seed(t, lower1, "a/", "c")

	s := newState(t, top, lower0, lower1)
	assertEntries(t, s, "", "a", "b", "c", "y", "z")

	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := s.readDir("")
	if err != nil {
		t.Fatalf("readDir: %v", err)
	}
	// b is a directory at the top and a file below it; a is a file at lowers[0]
	// and a directory below that.
	wantDir := map[string]bool{"a": false, "b": true, "c": false, "y": true, "z": false}
	for _, e := range entries {
		if e.IsDir() != wantDir[e.Name()] {
			t.Errorf("entry %q IsDir = %v, want %v", e.Name(), e.IsDir(), wantDir[e.Name()])
		}
	}
}

// TestOverlayStateNonDirectoryShadow checks that a non-directory in a layer
// closes the path the layers below it would be reached through, in the lookup
// and in the merged listing alike.
func TestOverlayStateNonDirectoryShadow(t *testing.T) {
	top, lower0, lower1 := threeLayers(t)
	seed(t, top, "blocked")
	seed(t, lower0, "blocked/child", "open/kept")
	seed(t, lower1, "open", "blocked/deeper")

	s := newState(t, top, lower0, lower1)

	assertLayer(t, s, "blocked", 0)
	assertLookupErr(t, s, "blocked/child", syscall.ENOTDIR)
	assertLookupErr(t, s, "blocked/deeper", syscall.ENOTDIR)

	assertLayer(t, s, "open", 1)
	assertEntries(t, s, "open", "kept")
}

// TestOverlayStateNonDirectoryBelowDirectory checks the other side of a blocked
// path: the merged view does have a directory there, contributed from above, so
// a name missing from it is absent rather than unreachable.
func TestOverlayStateNonDirectoryBelowDirectory(t *testing.T) {
	top, lower0, lower1 := threeLayers(t)
	seed(t, top, "dir/")
	seed(t, lower1, "dir")

	s := newState(t, top, lower0, lower1)

	assertLayer(t, s, "dir", 0)
	assertLookupErr(t, s, "dir/missing", fs.ErrNotExist)
	assertEntries(t, s, "dir")
}

// TestOverlayStateReadDirNonDirectory checks that readDir refuses a name the
// merged view does not show as a directory.
func TestOverlayStateReadDirNonDirectory(t *testing.T) {
	top, lower0, lower1 := threeLayers(t)
	seed(t, top, "file")
	seed(t, lower0, "file/child")

	s := newState(t, top, lower0, lower1)

	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, err := s.readDir("file"); !errors.Is(err, syscall.ENOTDIR) {
		t.Errorf("readDir(file) -> %v, want an error matching ENOTDIR", err)
	}
	if _, err := s.readDir("missing"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("readDir(missing) -> %v, want an error matching fs.ErrNotExist", err)
	}
}
