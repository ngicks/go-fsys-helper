package overlayfs

import (
	"io/fs"
	"testing"
	"time"
)

// TestChmodChtimesCopyUp checks the gate under the attribute setters: a lower's
// file is copied up and the change lands on the top's copy alone.
func TestChmodChtimesCopyUp(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, lower0, _ := newOverlay(t, mk)
		put(t, lower0, "file.txt", "lower", 0o644)
		lowerBefore, err := lower0.fsys.Stat(lower0.ContentPath("file.txt"))
		mustDo(t, "Stat lower", err)

		mustDo(t, "Chmod", f.Chmod("file.txt", 0o600))

		assertBackedBy(t, f, "file.txt", 0)
		assertMode(t, f, "file.txt", 0o600)
		lowerAfter, err := lower0.fsys.Stat(lower0.ContentPath("file.txt"))
		mustDo(t, "Stat lower", err)
		if lowerAfter.Mode().Perm() != lowerBefore.Mode().Perm() {
			t.Errorf("the lower's mode changed to %o, want %o left alone",
				lowerAfter.Mode().Perm(), lowerBefore.Mode().Perm())
		}
		if _, ok := layerContent(t, top, "file.txt"); !ok {
			t.Error("Chmod did not copy the file up")
		}

		when := time.Date(2020, 3, 4, 5, 6, 7, 0, time.UTC)
		mustDo(t, "Chtimes", f.Chtimes("file.txt", when, when))
		info, err := f.Stat("file.txt")
		mustDo(t, "Stat", err)
		if !info.ModTime().Equal(when) {
			t.Errorf("mtime after Chtimes = %v, want %v", info.ModTime(), when)
		}
		assertMarks(t, f, nil, nil)
	})
}

// TestMergedDirChmod checks that the directory handle's own Chmod passes the same
// gate: the directory it was opened on may have resolved to a lower.
func TestMergedDirChmod(t *testing.T) {
	f, top, lower0, _ := newOverlay(t, memfsMaker)
	put(t, lower0, "dir/inside.txt", "lower", 0o644)

	dir, err := f.Open("dir")
	mustDo(t, "Open", err)
	defer func() { _ = dir.Close() }()

	mustDo(t, "Chmod", dir.Chmod(0o700))

	assertDir(t, top.fsys, top.ContentPath("dir"))
	info, err := f.Stat("dir")
	mustDo(t, "Stat", err)
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("mode after the handle's Chmod = %o, want %o", got, 0o700)
	}
	// The handle answers for the top's copy the change landed on, not for the
	// lower directory it was opened on.
	info, err = dir.Stat()
	mustDo(t, "Stat through the handle", err)
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("mode through the handle after its own Chmod = %o, want %o", got, 0o700)
	}
	assertMarks(t, f, nil, nil)
}

// TestMergedDirChmodAfterClose checks that a closed directory handle changes
// nothing: both setters report fs.ErrClosed, the copy-up they would have gone
// through does not happen, and the lower directory keeps the mode it had.
func TestMergedDirChmodAfterClose(t *testing.T) {
	f, top, lower0, _ := newOverlay(t, memfsMaker)
	put(t, lower0, "dir/inside.txt", "lower", 0o644)
	before, err := lower0.fsys.Stat(lower0.ContentPath("dir"))
	mustDo(t, "Stat lower", err)

	dir, err := f.Open("dir")
	mustDo(t, "Open", err)
	mustDo(t, "Close", dir.Close())

	assertErrIs(t, "Chmod after Close", dir.Chmod(0o700), fs.ErrClosed)
	assertErrIs(t, "Chown after Close", dir.Chown(-1, -1), fs.ErrClosed)

	if exists(t, top.fsys, top.ContentPath("dir")) {
		t.Error("a closed handle's Chmod copied the directory up")
	}
	after, err := lower0.fsys.Stat(lower0.ContentPath("dir"))
	mustDo(t, "Stat lower", err)
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Errorf("the lower's mode changed to %o, want %o left alone",
			after.Mode().Perm(), before.Mode().Perm())
	}
	assertMarks(t, f, nil, nil)
}
