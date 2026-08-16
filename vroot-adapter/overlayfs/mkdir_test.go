package overlayfs

import (
	"io/fs"
	"path/filepath"
	"testing"
)

// TestMkdirOpaque checks the D7 rule where it is visible: a directory removed and
// made again shows none of the children the layers below still hold under it.
func TestMkdirOpaque(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, _, lower0, lower1 := newOverlay(t, mk)
		put(t, lower0, "dir/a.txt", "0", 0o644)
		put(t, lower1, "dir/b.txt", "1", 0o644)

		assertEntriesAt(t, f, "dir", "a.txt", "b.txt")

		mustDo(t, "RemoveAll", f.RemoveAll("dir"))
		assertNotExist(t, f, "dir")
		mustDo(t, "Mkdir", f.Mkdir("dir", 0o755))

		assertEntriesAt(t, f, "dir")
		assertMarks(t, f, nil, []string{"dir"})
		assertNotExist(t, f, "dir/a.txt")
		assertNotExist(t, f, "dir/b.txt")

		write(t, f, "dir/c.txt", "new")
		assertEntriesAt(t, f, "dir", "c.txt")
	})
}

// TestMkdirExisting checks that Mkdir refuses a name any layer already shows, not
// only one the top does.
func TestMkdirExisting(t *testing.T) {
	f, top, lower0, _ := newOverlay(t, memfsMaker)
	mkdirAt(t, top, "intop", 0o777)
	mkdirAt(t, lower0, "inlower", 0o777)

	assertErrIs(t, "Mkdir(intop)", f.Mkdir("intop", 0o777), fs.ErrExist)
	assertErrIs(t, "Mkdir(inlower)", f.Mkdir("inlower", 0o777), fs.ErrExist)
	assertErrIs(
		t,
		"Mkdir(missing/deep)",
		f.Mkdir(filepath.FromSlash("missing/deep"), 0o777),
		fs.ErrNotExist,
	)
}

// TestMkdirAllMixed checks MkdirAll over a path part of which is already there:
// the existing directories are left alone, masking included, and only the ones
// this call brings into being are opaque.
func TestMkdirAllMixed(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, _, lower0, _ := newOverlay(t, mk)
		put(t, lower0, "a/fromlower.txt", "0", 0o644)
		put(t, lower0, "a/b/deep.txt", "0", 0o644)

		mustDo(t, "MkdirAll", f.MkdirAll(filepath.FromSlash("a/b/c/d"), 0o755))

		assertMarks(t, f, nil, []string{"a/b/c", "a/b/c/d"})
		// Neither existing directory was disturbed, so what the layers below hold
		// under them is still merged in.
		assertEntriesAt(t, f, "a", "b", "fromlower.txt")
		assertEntriesAt(t, f, filepath.FromSlash("a/b"), "c", "deep.txt")
		assertEntriesAt(t, f, filepath.FromSlash("a/b/c"), "d")

		mustDo(t, "MkdirAll again", f.MkdirAll(filepath.FromSlash("a/b/c/d"), 0o755))
		assertMarks(t, f, nil, []string{"a/b/c", "a/b/c/d"})
	})
}

// TestMkdirAllBlocked checks the two paths MkdirAll cannot take: through a file,
// and onto one.
func TestMkdirAllBlocked(t *testing.T) {
	f, _, lower0, _ := newOverlay(t, memfsMaker)
	put(t, lower0, "stop.txt", "x", 0o644)

	if err := f.MkdirAll(filepath.FromSlash("stop.txt/below"), 0o755); err == nil {
		t.Error("MkdirAll through a file: want an error, got nil")
	}
	if err := f.MkdirAll("stop.txt", 0o755); err == nil {
		t.Error("MkdirAll onto a file: want an error, got nil")
	}
}
