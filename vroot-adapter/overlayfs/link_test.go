package overlayfs

import (
	"io/fs"
	"testing"
)

// TestLinkFromLower checks that a hard link to a name only a layer below holds
// copies the source up first: a hard link is two names for one file, which holds
// only inside one fs.
func TestLinkFromLower(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, lower0, _ := newOverlay(t, mk)
		put(t, lower0, "src.txt", "lower", 0o644)

		mustDo(t, "Link", f.Link("src.txt", "hard.txt"))

		assertBackedBy(t, f, "src.txt", 0)
		assertRead(t, f, "hard.txt", "lower")
		if _, ok := layerContent(t, top, "src.txt"); !ok {
			t.Error("the link source was not copied up")
		}
		assertMarks(t, f, nil, nil)

		// One file under two names: writing through one is read back through the
		// other.
		write(t, f, "hard.txt", "changed")
		assertRead(t, f, "src.txt", "changed")
	})
}

// TestLinkExisting checks that Link refuses a destination any layer shows and
// takes one a removal masked.
func TestLinkExisting(t *testing.T) {
	f, _, lower0, _ := newOverlay(t, memfsMaker)
	put(t, lower0, "src.txt", "lower", 0o644)
	put(t, lower0, "taken.txt", "lower", 0o644)

	assertErrIs(t, "Link onto an existing name", f.Link("src.txt", "taken.txt"), fs.ErrExist)
	assertErrIs(t, "Link from a missing name", f.Link("missing.txt", "new.txt"), fs.ErrNotExist)

	mustDo(t, "Remove", f.Remove("taken.txt"))
	mustDo(t, "Link over a whiteout", f.Link("src.txt", "taken.txt"))
	assertRead(t, f, "taken.txt", "lower")
	assertMarks(t, f, nil, nil)
}

// TestSymlinkOverWhiteout checks that a symlink put where a removal masked a name
// lifts the mask, and that nothing of the target is copied up — a symlink names a
// path and carries no content.
func TestSymlinkOverWhiteout(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, lower0, _ := newOverlay(t, mk)
		put(t, lower0, "target.txt", "lower", 0o644)
		put(t, lower0, "link", "lower", 0o644)
		mustDo(t, "Remove", f.Remove("link"))
		assertMarks(t, f, []string{"link"}, nil)

		mustDo(t, "Symlink", f.Symlink("target.txt", "link"))

		assertMarks(t, f, nil, nil)
		target, err := f.ReadLink("link")
		mustDo(t, "ReadLink", err)
		if target != "target.txt" {
			t.Errorf("ReadLink -> %q, want %q", target, "target.txt")
		}
		assertRead(t, f, "link", "lower")
		assertBackedBy(t, f, "target.txt", 1)
		if _, ok := layerContent(t, top, "target.txt"); ok {
			t.Error("the symlink target was copied up; a symlink names a path, not content")
		}
	})
}

// TestSymlinkExisting checks that Symlink refuses a name any layer shows.
func TestSymlinkExisting(t *testing.T) {
	f, _, lower0, _ := newOverlay(t, memfsMaker)
	put(t, lower0, "taken.txt", "lower", 0o644)

	assertErrIs(t, "Symlink onto an existing name", f.Symlink("x", "taken.txt"), fs.ErrExist)
}
