package overlayfs

import (
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
)

// TestRemoveTopOnly checks that a name no layer below shows is simply dropped:
// nothing is left to mask, and no row is written for it.
func TestRemoveTopOnly(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, _, _ := newOverlay(t, mk)
		write(t, f, "file.txt", "top")

		mustDo(t, "Remove", f.Remove("file.txt"))

		assertNotExist(t, f, "file.txt")
		if _, ok := layerContent(t, top, "file.txt"); ok {
			t.Error("the top still holds the removed file")
		}
		assertMarks(t, f, nil, nil)
	})
}

// TestRemoveLowerVisible checks that a name a layer below still shows is whited
// out rather than merely dropped, and that the whiteout is in the store by the
// time Remove returns (D14).
func TestRemoveLowerVisible(t *testing.T) {
	fsys := memfs.New("top")
	top := canonicalSource(t, fsys)
	lower0 := canonicalSource(t, memfs.New("lower0"))
	f, err := New(top, []*DataSource{lower0}, nil)
	mustDo(t, "New", err)

	put(t, lower0, "file.txt", "lower", 0o644)
	write(t, f, "file.txt", "top")
	assertBackedBy(t, f, "file.txt", 0)

	mustDo(t, "Remove", f.Remove("file.txt"))
	assertNotExist(t, f, "file.txt")
	assertMarks(t, f, []string{"file.txt"}, nil)
	if _, ok := layerContent(t, top, "file.txt"); ok {
		t.Error("the top still holds the removed file")
	}

	whiteouts, opaques := reopenedMarks(t, f, fsys)
	if !slices.Equal(whiteouts, []string{"file.txt"}) {
		t.Errorf("reopened store holds whiteouts %q, want %q", whiteouts, []string{"file.txt"})
	}
	if len(opaques) != 0 {
		t.Errorf("reopened store holds opaques %q, want none", opaques)
	}
}

// TestRemoveOverTopSelfMask checks the removal of a name the top holds a
// whiteout and content for at once, which is what a removal that recorded its
// mask and then failed to drop the file leaves behind: the mask stays, and the
// lower entry it hides does not come back.
func TestRemoveOverTopSelfMask(t *testing.T) {
	fsys := memfs.New("top")
	top := canonicalSource(t, fsys)
	lower0 := canonicalSource(t, memfs.New("lower0"))
	f, err := New(top, []*DataSource{lower0}, nil)
	mustDo(t, "New", err)

	put(t, lower0, "file.txt", "lower", 0o644)
	put(t, top, "file.txt", "top", 0o644)
	f.state.mu.Lock()
	mustDo(t, "setWhiteout", f.state.setWhiteout("file.txt"))
	f.state.mu.Unlock()

	mustDo(t, "Remove", f.Remove("file.txt"))

	assertNotExist(t, f, "file.txt")
	assertMarks(t, f, []string{"file.txt"}, nil)
	if _, ok := layerContent(t, top, "file.txt"); ok {
		t.Error("the top still holds the removed file")
	}

	whiteouts, _ := reopenedMarks(t, f, fsys)
	if !slices.Equal(whiteouts, []string{"file.txt"}) {
		t.Errorf("reopened store holds whiteouts %q, want %q", whiteouts, []string{"file.txt"})
	}
}

// TestRemoveNonEmptyMerged checks that emptiness is judged over the merged view:
// a directory the top holds nothing in is still full when a layer below fills it.
func TestRemoveNonEmptyMerged(t *testing.T) {
	f, _, lower0, _ := newOverlay(t, memfsMaker)
	put(t, lower0, "dir/inside.txt", "lower", 0o644)
	mustDo(t, "Mkdir", f.MkdirAll("other", 0o777))

	assertErrIs(t, "Remove(dir)", f.Remove("dir"), errdef.ENOTEMPTY)
	mustDo(t, "Remove(other)", f.Remove("other"))

	mustDo(t, "Remove(dir/inside.txt)", f.Remove(filepath.FromSlash("dir/inside.txt")))
	mustDo(t, "Remove(dir)", f.Remove("dir"))
	assertNotExist(t, f, "dir")
}

// TestRemoveMissing checks that Remove reports a name that is not there, and one
// a removal already masked, the way a plain fs does.
func TestRemoveMissing(t *testing.T) {
	f, _, lower0, _ := newOverlay(t, memfsMaker)
	put(t, lower0, "file.txt", "lower", 0o644)

	assertErrIs(t, "Remove(missing)", f.Remove("missing"), fs.ErrNotExist)
	mustDo(t, "Remove", f.Remove("file.txt"))
	assertErrIs(t, "Remove(whiteouted)", f.Remove("file.txt"), fs.ErrNotExist)
}

// TestRemoveAllMixedTree checks RemoveAll over a tree the top and the layers
// below both fill: one whiteout masks the whole of it, the rows that described
// what was inside are gone, and the store says so after a reopen.
func TestRemoveAllMixedTree(t *testing.T) {
	fsys := memfs.New("top")
	top := canonicalSource(t, fsys)
	lower0 := canonicalSource(t, memfs.New("lower0"))
	f, err := New(top, []*DataSource{lower0}, nil)
	mustDo(t, "New", err)

	put(t, lower0, "tree/fromlower.txt", "0", 0o644)
	put(t, lower0, "tree/sub/deep.txt", "0", 0o644)
	put(t, lower0, "kept/stays.txt", "0", 0o644)
	write(t, f, "tree/fromtop.txt", "top")
	mustDo(t, "Mkdir", f.Mkdir(filepath.FromSlash("tree/fresh"), 0o755))
	mustDo(t, "Remove", f.Remove(filepath.FromSlash("tree/sub/deep.txt")))
	assertMarks(t, f, []string{"tree/sub/deep.txt"}, []string{"tree/fresh"})

	mustDo(t, "RemoveAll", f.RemoveAll("tree"))

	assertNotExist(t, f, "tree")
	assertMarks(t, f, []string{"tree"}, nil)
	assertEntriesAt(t, f, ".", "kept")
	if _, ok := layerContent(t, top, "tree/fromtop.txt"); ok {
		t.Error("the top still holds a file under the removed tree")
	}

	whiteouts, opaques := reopenedMarks(t, f, fsys)
	if !slices.Equal(whiteouts, []string{"tree"}) {
		t.Errorf("reopened store holds whiteouts %q, want %q", whiteouts, []string{"tree"})
	}
	if len(opaques) != 0 {
		t.Errorf("reopened store holds opaques %q, want none", opaques)
	}
}

// TestRemoveAllMissing checks that RemoveAll reports success for what is not
// there, whichever way it is missing.
func TestRemoveAllMissing(t *testing.T) {
	f, _, lower0, _ := newOverlay(t, memfsMaker)
	put(t, lower0, "file.txt", "lower", 0o644)

	mustDo(t, "RemoveAll(missing)", f.RemoveAll("missing"))
	mustDo(t, "RemoveAll(missing/deep)", f.RemoveAll(filepath.FromSlash("missing/deep")))
	mustDo(t, "RemoveAll", f.RemoveAll("file.txt"))
	mustDo(t, "RemoveAll again", f.RemoveAll("file.txt"))
}

// TestDisableOpenFileRemoval checks the Windows-style refusal: a name the overlay
// still has a handle out on cannot be removed while the option is set.
func TestDisableOpenFileRemoval(t *testing.T) {
	top := canonicalSource(t, memfs.New("top"))
	lower0 := canonicalSource(t, memfs.New("lower0"))
	put(t, lower0, "file.txt", "lower", 0o644)
	put(t, lower0, "other.txt", "lower", 0o644)
	f, err := New(top, []*DataSource{lower0}, &Option{DisableOpenFileRemoval: true})
	mustDo(t, "New", err)

	file, err := f.Open("file.txt")
	mustDo(t, "Open", err)

	assertErrIs(t, "Remove with a handle out", f.Remove("file.txt"), errSharingViolation)
	assertErrIs(t, "RemoveAll with a handle out", f.RemoveAll("file.txt"), errSharingViolation)
	assertExists(t, f, "file.txt")
	assertMarks(t, f, nil, nil)

	mustDo(t, "Remove(other)", f.Remove("other.txt"))

	mustDo(t, "Close", file.Close())
	mustDo(t, "Remove after close", f.Remove("file.txt"))
	assertNotExist(t, f, "file.txt")
}
