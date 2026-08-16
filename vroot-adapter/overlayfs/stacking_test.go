package overlayfs_test

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs"
)

func canonicalOver(t *testing.T, fsys vroot.Fs[vroot.File]) *overlayfs.DataSource {
	t.Helper()
	d, err := overlayfs.NewDataSourceCanonical[vroot.File](fsys)
	if err != nil {
		t.Fatalf("NewDataSourceCanonical: %v", err)
	}
	return d
}

func stackOver(
	t *testing.T,
	top *overlayfs.DataSource,
	lowers ...*overlayfs.DataSource,
) *overlayfs.Fs {
	t.Helper()
	f, err := overlayfs.New(top, lowers, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f
}

func readAll(t *testing.T, fsys vroot.Fs[vroot.File], name string) string {
	t.Helper()
	b, err := vroot.ReadFile(fsys, filepath.FromSlash(name))
	if err != nil {
		t.Fatalf("readfile %q: %v", name, err)
	}
	return string(b)
}

func assertContent(t *testing.T, fsys vroot.Fs[vroot.File], name string, want string) {
	t.Helper()
	if got := readAll(t, fsys, name); got != want {
		t.Errorf("read %q = %q, want %q", name, got, want)
	}
}

func assertGone(t *testing.T, fsys vroot.Fs[vroot.File], name string) {
	t.Helper()
	if _, err := fsys.Stat(filepath.FromSlash(name)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stat %q = %v, want a not-exist error", name, err)
	}
}

func dirNames(t *testing.T, fsys vroot.Fs[vroot.File], name string) []string {
	t.Helper()
	entries, err := vroot.ReadDir(fsys, filepath.FromSlash(name))
	if err != nil {
		t.Fatalf("readdir %q: %v", name, err)
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	slices.Sort(names)
	return names
}

func assertDirNames(t *testing.T, fsys vroot.Fs[vroot.File], name string, want ...string) {
	t.Helper()
	if got := dirNames(t, fsys, name); !slices.Equal(got, want) {
		t.Errorf("entries of %q = %q, want %q", name, got, want)
	}
}

// TestCanonicalLowerStacking runs the second use case IDEA describes: an
// overlay's top is stopped, and the directory it wrote is stacked as a lower of
// the next overlay.
//
// What the first overlay recorded has to survive that unchanged — its copies,
// its whiteouts and its opaque directories — and go on masking the layer it was
// stacked over, without the layer itself being written to again.
func TestCanonicalLowerStacking(t *testing.T) {
	for _, s := range stacks() {
		t.Run(s.name, func(t *testing.T) {
			lowerFsys := s.fsys(t, "lower")
			t1Fsys := s.fsys(t, "top1")
			t2Fsys := s.fsys(t, "top2")
			t3Fsys := s.fsys(t, "top3")

			testhelper.New[*testing.T, vroot.File](t, lowerFsys).SetupLines(
				`keep.txt: "L-keep"`,
				`edit.txt: "L-edit"`,
				`drop.txt: "L-drop"`,
				"cut/",
				`cut/deep.txt: "L-deep"`,
				`cut/other.txt: "L-other"`,
			)

			// Overlay A: every kind of change the top can record.
			a := stackOver(t,
				canonicalOver(t, t1Fsys),
				overlayfs.NewDataSource[vroot.File](lowerFsys),
			)
			testhelper.New(t, a).SetupLines(
				`edit.txt: "A-edit"`,
				`fresh.txt: "A-fresh"`,
			)
			if err := a.Remove(filepath.FromSlash("drop.txt")); err != nil {
				t.Fatalf("Remove: %v", err)
			}
			// Removed before it is re-made: Mkdir refuses a name the lower still
			// shows, and it is the re-made directory that carries the opaque mark.
			if err := a.RemoveAll(filepath.FromSlash("cut")); err != nil {
				t.Fatalf("RemoveAll: %v", err)
			}
			testhelper.New(t, a).SetupLines(
				"cut/",
				`cut/mine.txt: "A-mine"`,
			)
			if err := a.Close(); err != nil {
				t.Fatalf("Close(A): %v", err)
			}

			// Overlay B: A's top stacked as the canonical lower it now is.
			b := stackOver(t,
				canonicalOver(t, t2Fsys),
				canonicalOver(t, t1Fsys),
				overlayfs.NewDataSource[vroot.File](lowerFsys),
			)

			assertContent(t, b, "keep.txt", "L-keep")
			assertContent(t, b, "edit.txt", "A-edit")
			assertContent(t, b, "fresh.txt", "A-fresh")
			assertGone(t, b, "drop.txt")
			assertDirNames(t, b, "cut", "mine.txt")

			testhelper.New(t, b).SetupLines(
				`new-in-b.txt: "B-new"`,
				// A name A whiteouted, written again: the mark a lower carries masks
				// what is under that lower, never the top written over it.
				`drop.txt: "B-drop"`,
			)
			assertContent(t, b, "drop.txt", "B-drop")

			assertContent(t, t2Fsys, filepath.Join("merged", "new-in-b.txt"), "B-new")
			assertContent(t, t2Fsys, filepath.Join("merged", "drop.txt"), "B-drop")
			for _, name := range []string{"new-in-b.txt", "drop.txt"} {
				assertGone(t, t1Fsys, filepath.Join("merged", name))
			}
			assertContent(t, t1Fsys, filepath.Join("merged", "edit.txt"), "A-edit")
			assertDirNames(t, t1Fsys, filepath.Join("merged", "cut"), "mine.txt")

			if err := b.Close(); err != nil {
				t.Fatalf("Close(B): %v", err)
			}

			// Overlay C: A's top a second time, and this time over a backend that
			// refuses the write, so its store is the read-only one a frozen layer
			// gets. What it masks has to be the same either way.
			frozen, err := overlayfs.NewDataSourceCanonical[*vroot.ReadOnlyFile](
				vroot.NewReadOnlyFs[vroot.File](t1Fsys),
			)
			if err != nil {
				t.Fatalf("NewDataSourceCanonical(read-only): %v", err)
			}
			c := stackOver(t,
				canonicalOver(t, t3Fsys),
				frozen,
				overlayfs.NewDataSource[vroot.File](lowerFsys),
			)

			assertContent(t, c, "edit.txt", "A-edit")
			assertContent(t, c, "keep.txt", "L-keep")
			assertGone(t, c, "drop.txt")
			assertDirNames(t, c, "cut", "mine.txt")

			if err := c.Close(); err != nil {
				t.Fatalf("Close(C): %v", err)
			}

			// What A left in the store, read out of it once every overlay that held
			// it is gone: the masking the three stacks above went by, in the form it
			// is kept in rather than the shape a lookup reports.
			store, err := overlayfs.NewMetadataStoreSQLite[vroot.File](t1Fsys, storeName)
			if err != nil {
				t.Fatalf("NewMetadataStoreSQLite: %v", err)
			}
			defer func() { _ = store.Close() }()
			whiteouts, opaques, err := store.Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			slices.Sort(whiteouts)
			slices.Sort(opaques)
			if !slices.Equal(whiteouts, []string{"drop.txt"}) {
				t.Errorf("whiteouts = %q, want %q", whiteouts, []string{"drop.txt"})
			}
			if !slices.Equal(opaques, []string{"cut"}) {
				t.Errorf("opaques = %q, want %q", opaques, []string{"cut"})
			}
		})
	}
}
