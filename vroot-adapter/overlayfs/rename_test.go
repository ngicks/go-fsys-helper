package overlayfs

import (
	"io/fs"
	"path/filepath"
	"slices"
	"syscall"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
)

// TestRenameFile checks both sources: one the top already holds, and one it has
// to copy up first. Either way the old name is whited out only when a layer
// below still shows it.
func TestRenameFile(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		t.Run("top-backed", func(t *testing.T) {
			f, _, _, _ := newOverlay(t, mk)
			write(t, f, "old.txt", "content")

			mustDo(t, "Rename", f.Rename("old.txt", "new.txt"))

			assertNotExist(t, f, "old.txt")
			assertRead(t, f, "new.txt", "content")
			assertMarks(t, f, nil, nil)
		})

		t.Run("lower-backed", func(t *testing.T) {
			f, top, lower0, _ := newOverlay(t, mk)
			put(t, lower0, "old.txt", "lower", 0o644)

			mustDo(t, "Rename", f.Rename("old.txt", "new.txt"))

			assertNotExist(t, f, "old.txt")
			assertRead(t, f, "new.txt", "lower")
			assertBackedBy(t, f, "new.txt", 0)
			assertMarks(t, f, []string{"old.txt"}, nil)
			if got, ok := layerContent(t, lower0, "old.txt"); !ok || got != "lower" {
				t.Errorf("lower holds %q (present=%v) after the rename, want %q", got, ok, "lower")
			}
			if _, ok := layerContent(t, top, "new.txt"); !ok {
				t.Error("the renamed file did not land in the top")
			}
		})
	})
}

// TestRenameOntoWhiteout checks a rename onto a name a removal masked: the mask
// comes off with the content that lands there.
func TestRenameOntoWhiteout(t *testing.T) {
	f, _, lower0, _ := newOverlay(t, memfsMaker)
	put(t, lower0, "target.txt", "lower", 0o644)
	write(t, f, "source.txt", "source")
	mustDo(t, "Remove", f.Remove("target.txt"))
	assertMarks(t, f, []string{"target.txt"}, nil)

	mustDo(t, "Rename", f.Rename("source.txt", "target.txt"))

	assertRead(t, f, "target.txt", "source")
	assertNotExist(t, f, "source.txt")
	assertMarks(t, f, nil, nil)

	// The lower's copy must stay masked by the top's file rather than by a row:
	// removing the top's copy is what puts a whiteout back.
	mustDo(t, "Remove", f.Remove("target.txt"))
	assertNotExist(t, f, "target.txt")
	assertMarks(t, f, []string{"target.txt"}, nil)
}

// TestRenameOntoLowerVisible checks that a rename onto a name only a layer below
// shows needs no whiteout of its own: the top now holds content there, and
// content in the top shadows the layers below.
func TestRenameOntoLowerVisible(t *testing.T) {
	f, _, lower0, _ := newOverlay(t, memfsMaker)
	put(t, lower0, "target.txt", "lower", 0o644)
	write(t, f, "source.txt", "source")

	mustDo(t, "Rename", f.Rename("source.txt", "target.txt"))

	assertRead(t, f, "target.txt", "source")
	assertMarks(t, f, nil, nil)

	mustDo(t, "Remove", f.Remove("target.txt"))
	assertNotExist(t, f, "target.txt")
	assertMarks(t, f, []string{"target.txt"}, nil)
}

// TestRenameDirCrossDevice pins the refusal the reference makes here: a directory
// the layers below contribute to cannot be moved, since moving it would mean
// copying its whole lower subtree up or leaving a redirect behind for the old
// path (Rust overlay.rs:2251-2265; C main.c:4756-4763 @ 4759abd).
func TestRenameDirCrossDevice(t *testing.T) {
	t.Run("directory in a lower", func(t *testing.T) {
		f, _, lower0, _ := newOverlay(t, memfsMaker)
		put(t, lower0, "dir/inside.txt", "lower", 0o644)

		err := f.Rename("dir", "moved")
		assertErrIs(t, "Rename(lower-backed dir)", err, errCrossDevice)
		assertExists(t, f, "dir")
		assertNotExist(t, f, "moved")
	})

	t.Run("top directory a lower still fills", func(t *testing.T) {
		f, top, lower0, _ := newOverlay(t, memfsMaker)
		put(t, lower0, "dir/fromlower.txt", "lower", 0o644)
		mkdirAt(t, top, "dir", 0o777)
		write(t, f, "dir/fromtop.txt", "top")

		err := f.Rename("dir", "moved")
		assertErrIs(t, "Rename(dir a lower fills)", err, errCrossDevice)
	})

	t.Run("top directory masking its lower", func(t *testing.T) {
		f, _, lower0, _ := newOverlay(t, memfsMaker)
		put(t, lower0, "dir/fromlower.txt", "lower", 0o644)
		mustDo(t, "RemoveAll", f.RemoveAll("dir"))
		mustDo(t, "Mkdir", f.Mkdir("dir", 0o755))
		write(t, f, "dir/fromtop.txt", "top")

		// Opaque cuts the layers below off, so nothing of theirs has to move.
		mustDo(t, "Rename", f.Rename("dir", "moved"))
		assertEntriesAt(t, f, "moved", "fromtop.txt")
		assertNotExist(t, f, "dir/fromlower.txt")
	})
}

// TestRenameDirMissingWhiteouts checks the create_missing_whiteouts analogue: a
// directory moved onto a name the layers below fill must not pick their entries
// up, which one opaque mark on the new name settles for the whole subtree.
func TestRenameDirMissingWhiteouts(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, _, lower0, _ := newOverlay(t, mk)
		put(t, lower0, "dst/fromlower.txt", "0", 0o644)
		put(t, lower0, "dst/sub/deep.txt", "0", 0o644)
		mustDo(t, "Mkdir", f.Mkdir("src", 0o755))
		write(t, f, "src/fromtop.txt", "top")
		mustDo(t, "Remove", f.Remove(filepath.FromSlash("dst/fromlower.txt")))
		mustDo(t, "Remove", f.Remove(filepath.FromSlash("dst/sub/deep.txt")))
		mustDo(t, "Remove", f.Remove(filepath.FromSlash("dst/sub")))
		mustDo(t, "Remove", f.Remove("dst"))

		mustDo(t, "Rename", f.Rename("src", "dst"))

		assertEntriesAt(t, f, "dst", "fromtop.txt")
		assertNotExist(t, f, "dst/fromlower.txt")
		assertNotExist(t, f, filepath.FromSlash("dst/sub"))
		assertNotExist(t, f, filepath.FromSlash("dst/sub/deep.txt"))
		// The whiteouts the removals left under the destination are gone: one
		// opaque mark says the same about the whole subtree, the names the moved
		// directory provides included.
		assertMarks(t, f, nil, []string{"dst"})
	})
}

// TestRenameDirMigratesMarks checks that the rows describing what is masked
// inside a moved directory move with it. The reference gets this for free — its
// whiteouts are files inside the directory — while rows keyed by full path have
// to be rewritten (D19).
func TestRenameDirMigratesMarks(t *testing.T) {
	fsys := memfs.New("top")
	top := canonicalSource(t, fsys)
	lower0 := canonicalSource(t, memfs.New("lower0"))
	f, err := New(top, []*DataSource{lower0}, nil)
	mustDo(t, "New", err)

	put(t, lower0, "src/gone.txt", "0", 0o644)
	put(t, lower0, "src/sub/gone.txt", "0", 0o644)
	// Opaque on src cuts the layers below off, which is what lets the directory
	// move at all; the rows under it stay behind to be migrated.
	mustDo(t, "RemoveAll", f.RemoveAll("src"))
	mustDo(t, "Mkdir", f.Mkdir("src", 0o755))
	mustDo(t, "MkdirAll", f.MkdirAll(filepath.FromSlash("src/sub/deeper"), 0o755))
	write(t, f, "src/sub/kept.txt", "top")
	assertMarks(t, f, nil, []string{"src", "src/sub", "src/sub/deeper"})

	mustDo(t, "Rename", f.Rename("src", "dst"))

	// src is whited out because the layer below still shows a directory there:
	// the opaque mark that hid its children moved away with the top's directory.
	assertMarks(t, f, []string{"src"}, []string{"dst", "dst/sub", "dst/sub/deeper"})
	assertNotExist(t, f, "src")
	assertRead(t, f, filepath.FromSlash("dst/sub/kept.txt"), "top")
	assertEntriesAt(t, f, filepath.FromSlash("dst/sub"), "deeper", "kept.txt")
	// The destination shows nothing of what the layer below holds at src.
	assertEntriesAt(t, f, "dst", "sub")

	whiteouts, opaques := reopenedMarks(t, f, fsys)
	if !slices.Equal(whiteouts, []string{"src"}) {
		t.Errorf("reopened store holds whiteouts %q, want %q", whiteouts, []string{"src"})
	}
	want := []string{"dst", "dst/sub", "dst/sub/deeper"}
	if !slices.Equal(opaques, want) {
		t.Errorf("reopened store holds opaques %q, want %q", opaques, want)
	}
}

// TestRenameOntoNonEmpty checks that a rename onto a directory the merged view
// still shows entries in is refused, even when every one of them comes from a
// layer below.
func TestRenameOntoNonEmpty(t *testing.T) {
	f, _, lower0, _ := newOverlay(t, memfsMaker)
	put(t, lower0, "dst/fromlower.txt", "0", 0o644)
	mustDo(t, "Mkdir", f.Mkdir("src", 0o755))

	assertErrIs(t, "Rename onto a non-empty merged dir", f.Rename("src", "dst"), errdef.ENOTEMPTY)
	assertExists(t, f, "src")
}

// TestRenameRefusals checks the shapes rename(2) itself refuses.
func TestRenameRefusals(t *testing.T) {
	f, _, lower0, _ := newOverlay(t, memfsMaker)
	put(t, lower0, "file.txt", "lower", 0o644)
	mustDo(t, "MkdirAll", f.MkdirAll(filepath.FromSlash("dir/inner"), 0o755))

	assertErrIs(t, "Rename(missing)", f.Rename("missing", "anything"), fs.ErrNotExist)
	assertErrIs(t, "Rename into own subtree",
		f.Rename("dir", filepath.FromSlash("dir/inner/below")), syscall.EINVAL)
	assertErrIs(t, "Rename file onto dir", f.Rename("file.txt", "dir"), syscall.EISDIR)
	assertErrIs(t, "Rename dir onto file", f.Rename("dir", "file.txt"), syscall.ENOTDIR)
	mustDo(t, "Rename onto itself", f.Rename("file.txt", "file.txt"))
}
