package overlayfs

import (
	"errors"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// write puts content at name through the overlay, which is what sends it through
// the copy-up gate.
func write(t *testing.T, f *Fs, name string, content string) {
	t.Helper()
	file, err := f.Create(filepath.FromSlash(name))
	if err != nil {
		t.Fatalf("create %q: %v", name, err)
	}
	if _, err := file.Write([]byte(content)); err != nil {
		_ = file.Close()
		t.Fatalf("write %q: %v", name, err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close %q: %v", name, err)
	}
}

func mustDo(t *testing.T, op string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", op, err)
	}
}

// assertBackedBy checks which layer of the stack answers name.
func assertBackedBy(t *testing.T, f *Fs, name string, want int) {
	t.Helper()
	f.state.mu.RLock()
	defer f.state.mu.RUnlock()
	res, err := f.state.lookup(cleanName(name))
	if err != nil {
		t.Errorf("lookup %q: %v, want layer %d", name, err, want)
		return
	}
	if res.layer != want {
		t.Errorf("lookup %q -> layer %d, want %d", name, res.layer, want)
	}
}

func assertNotExist(t *testing.T, f *Fs, name string) {
	t.Helper()
	if _, err := f.Lstat(filepath.FromSlash(name)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Lstat %q -> %v, want an error matching fs.ErrNotExist", name, err)
	}
}

func assertExists(t *testing.T, f *Fs, name string) {
	t.Helper()
	if _, err := f.Lstat(filepath.FromSlash(name)); err != nil {
		t.Errorf("Lstat %q: %v, want it to exist", name, err)
	}
}

// topMarks reports what the top layer holds in memory.
func topMarks(f *Fs) (whiteouts []string, opaques []string) {
	f.state.mu.RLock()
	defer f.state.mu.RUnlock()
	return slices.Sorted(maps.Keys(f.state.masks[0].whiteout)),
		slices.Sorted(maps.Keys(f.state.masks[0].opaque))
}

func assertMarks(t *testing.T, f *Fs, wantWhiteout []string, wantOpaque []string) {
	t.Helper()
	whiteouts, opaques := topMarks(f)
	if !slices.Equal(whiteouts, wantWhiteout) {
		t.Errorf("whiteouts = %q, want %q", whiteouts, wantWhiteout)
	}
	if !slices.Equal(opaques, wantOpaque) {
		t.Errorf("opaques = %q, want %q", opaques, wantOpaque)
	}
}

// reopenedMarks closes the overlay and reads the top's store back through a
// fresh data source over the same fs, which is what shows a masking change was
// written through rather than only remembered (D14).
func reopenedMarks(
	t *testing.T,
	f *Fs,
	fsys vroot.Fs[vroot.File],
) (whiteouts []string, opaques []string) {
	t.Helper()
	mustDo(t, "Close", f.Close())
	reopened, err := NewDataSourceCanonical(fsys)
	if err != nil {
		t.Fatalf("NewDataSourceCanonical after close: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	whiteouts, opaques, err = reopened.store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	slices.Sort(whiteouts)
	slices.Sort(opaques)
	return whiteouts, opaques
}

// entries lists what the merged view shows in the directory name.
func entries(t *testing.T, f *Fs, name string) []string {
	t.Helper()
	dir, err := f.Open(filepath.FromSlash(name))
	if err != nil {
		t.Fatalf("open %q: %v", name, err)
	}
	defer func() { _ = dir.Close() }()
	names, err := dir.Readdirnames(-1)
	if err != nil {
		t.Fatalf("readdirnames %q: %v", name, err)
	}
	slices.Sort(names)
	return names
}

func assertEntriesAt(t *testing.T, f *Fs, name string, want ...string) {
	t.Helper()
	if got := entries(t, f, name); !slices.Equal(got, want) {
		t.Errorf("entries of %q = %q, want %q", name, got, want)
	}
}

// layerContent reads name straight out of a layer's content tree, bypassing the
// merged view, which is how a lower is checked for having been left alone.
func layerContent(t *testing.T, d *DataSource, name string) (string, bool) {
	t.Helper()
	b, err := vroot.ReadFile(d.fsys, d.ContentPath(name))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatalf("read %q from %q: %v", name, d.fsys.Name(), err)
	}
	return string(b), true
}

// TestWriteCopiesUp checks the gate itself: writing to a file only a lower holds
// brings it into the top, leaves the lower as it was, and records no masking —
// a copy-up moves where a name is read from, not whether it is visible.
func TestWriteCopiesUp(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, lower0, _ := newOverlay(t, mk)
		put(t, lower0, "dir/file.txt", "lower", 0o644)

		assertBackedBy(t, f, "dir/file.txt", 1)
		write(t, f, "dir/file.txt", "top")

		assertBackedBy(t, f, "dir/file.txt", 0)
		assertRead(t, f, filepath.FromSlash("dir/file.txt"), "top")
		if got, ok := layerContent(t, top, "dir/file.txt"); !ok || got != "top" {
			t.Errorf("top holds %q (present=%v), want %q", got, ok, "top")
		}
		if got, ok := layerContent(t, lower0, "dir/file.txt"); !ok || got != "lower" {
			t.Errorf("lower holds %q (present=%v) after the copy-up, want %q", got, ok, "lower")
		}
		assertMarks(t, f, nil, nil)
	})
}

// TestWriteCopiesUpParents checks that the directories a copy-up has to create on
// the way stay merged with the layers below: they were made in passing, not
// created for their own sake, so they carry no opaque mark (D7).
func TestWriteCopiesUpParents(t *testing.T) {
	f, top, lower0, _ := newOverlay(t, memfsMaker)
	put(t, lower0, "dir/kept.txt", "lower", 0o644)
	put(t, lower0, "dir/moved.txt", "lower", 0o644)

	write(t, f, "dir/moved.txt", "top")

	assertDir(t, top.fsys, top.ContentPath("dir"))
	assertMarks(t, f, nil, nil)
	assertEntriesAt(t, f, ".", "dir")
	assertEntriesAt(t, f, "dir", "kept.txt", "moved.txt")
	assertRead(t, f, filepath.FromSlash("dir/kept.txt"), "lower")
}

// TestOpenFileCreateMatrix checks what O_CREATE and O_EXCL make of a name the
// merged view shows, one it does not, and one a removal masked.
func TestOpenFileCreateMatrix(t *testing.T) {
	t.Run("O_EXCL over a lower-visible file", func(t *testing.T) {
		f, _, lower0, _ := newOverlay(t, memfsMaker)
		put(t, lower0, "file.txt", "lower", 0o644)

		_, err := f.OpenFile("file.txt", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
		assertErrIs(t, "OpenFile(O_EXCL)", err, fs.ErrExist)
		assertBackedBy(t, f, "file.txt", 1)
	})

	t.Run("O_EXCL over a whiteouted name", func(t *testing.T) {
		f, _, lower0, _ := newOverlay(t, memfsMaker)
		put(t, lower0, "file.txt", "lower", 0o644)
		mustDo(t, "Remove", f.Remove("file.txt"))
		assertMarks(t, f, []string{"file.txt"}, nil)

		file, err := f.OpenFile("file.txt", os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
		mustDo(t, "OpenFile(O_EXCL) over a whiteout", err)
		mustDo(t, "Close", file.Close())

		assertBackedBy(t, f, "file.txt", 0)
		assertRead(t, f, "file.txt", "")
		assertMarks(t, f, nil, nil)
	})

	t.Run("no O_CREATE on a missing name", func(t *testing.T) {
		f, _, _, _ := newOverlay(t, memfsMaker)
		_, err := f.OpenFile("missing.txt", os.O_WRONLY, 0)
		assertErrIs(t, "OpenFile(O_WRONLY)", err, fs.ErrNotExist)
	})

	t.Run("write open of a directory", func(t *testing.T) {
		f, _, lower0, _ := newOverlay(t, memfsMaker)
		mkdirAt(t, lower0, "dir", 0o777)
		_, err := f.OpenFile("dir", os.O_WRONLY, 0)
		assertErrIs(t, "OpenFile(dir, O_WRONLY)", err, syscall.EISDIR)
	})

	t.Run("O_TRUNC over a lower file", func(t *testing.T) {
		f, top, lower0, _ := newOverlay(t, memfsMaker)
		put(t, lower0, "file.txt", "lower", 0o644)

		file, err := f.OpenFile("file.txt", os.O_RDWR|os.O_TRUNC, 0o666)
		mustDo(t, "OpenFile(O_TRUNC)", err)
		mustDo(t, "Close", file.Close())

		assertRead(t, f, "file.txt", "")
		if got, ok := layerContent(t, lower0, "file.txt"); !ok || got != "lower" {
			t.Errorf("lower holds %q (present=%v) after O_TRUNC, want %q", got, ok, "lower")
		}
		if _, ok := layerContent(t, top, "file.txt"); !ok {
			t.Error("the truncated file did not land in the top")
		}
	})

	t.Run("create under a whiteouted directory", func(t *testing.T) {
		f, _, lower0, _ := newOverlay(t, memfsMaker)
		put(t, lower0, "dir/file.txt", "lower", 0o644)
		mustDo(t, "RemoveAll", f.RemoveAll("dir"))

		_, err := f.Create(filepath.FromSlash("dir/file.txt"))
		assertErrIs(t, "Create under a removed directory", err, fs.ErrNotExist)
	})
}

// TestStructuralOpsRefuseRoot checks that the view's own root is not a target a
// structural op can take, in the overlay and in a sub-overlay confined to a
// directory inside it.
func TestStructuralOpsRefuseRoot(t *testing.T) {
	f, _, lower0, _ := newOverlay(t, memfsMaker)
	put(t, lower0, "dir/inside.txt", "lower", 0o644)
	sub, err := f.OpenRoot("dir")
	mustDo(t, "OpenRoot", err)

	for name, view := range map[string]*Fs{"overlay": f, "sub-overlay": sub} {
		assertErrIs(t, name+" Remove(.)", view.Remove("."), syscall.EINVAL)
		assertErrIs(t, name+" RemoveAll(.)", view.RemoveAll("."), syscall.EINVAL)
		assertErrIs(t, name+" Rename(., x)", view.Rename(".", "x"), syscall.EINVAL)
		assertErrIs(t, name+" Rename(x, .)", view.Rename("x", "."), syscall.EINVAL)
	}
	assertExists(t, f, "dir")
	assertExists(t, f, filepath.FromSlash("dir/inside.txt"))
}

// TestNoMarkerFilesInLayers walks every layer after a run of the mutations the
// suite exercises and asserts none of them wrote a masking artifact into a layer:
// no ".wh." file, no opaque sentinel, nothing but the content and the canonical
// layout (D6). It holds by construction — masking has nowhere to go but the
// store — which is what makes it worth pinning.
func TestNoMarkerFilesInLayers(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, lower0, lower1 := newOverlay(t, mk)

		put(t, lower0, "dir/fromlower.txt", "0", 0o644)
		put(t, lower0, "dir/sub/deep.txt", "0", 0o644)
		put(t, lower1, "dir/deeper.txt", "1", 0o644)
		put(t, lower1, "solo.txt", "1", 0o644)

		write(t, f, "dir/fromtop.txt", "top")
		write(t, f, "dir/fromlower.txt", "copied up")
		mustDo(t, "Remove", f.Remove(filepath.FromSlash("dir/deeper.txt")))
		mustDo(t, "RemoveAll", f.RemoveAll(filepath.FromSlash("dir/sub")))
		mustDo(t, "MkdirAll", f.MkdirAll(filepath.FromSlash("fresh/inner"), 0o755))
		mustDo(t, "Symlink", f.Symlink("solo.txt", "alias"))
		mustDo(t, "Link", f.Link("solo.txt", "hard.txt"))
		mustDo(t, "Rename", f.Rename("hard.txt", filepath.FromSlash("fresh/moved.txt")))
		mustDo(t, "Rename dir", f.Rename(filepath.FromSlash("fresh/inner"), "elsewhere"))
		mustDo(t, "Chmod", f.Chmod("solo.txt", 0o600))
		mustDo(t, "Remove", f.Remove("solo.txt"))

		for _, d := range []*DataSource{top, lower0, lower1} {
			assertNoMarkers(t, d)
		}
	})
}

// assertNoMarkers walks a layer's whole fs and fails on any name that reads as a
// masking artifact. Directory-entry names are checked, not paths, so a name
// anywhere in the tree is caught.
func assertNoMarkers(t *testing.T, d *DataSource) {
	t.Helper()
	err := fs.WalkDir(
		vroot.ToIoFs(d.fsys),
		".",
		func(p string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			name := path.Base(p)
			if p == "." {
				return nil
			}
			switch {
			case strings.HasPrefix(name, ".wh."):
				t.Errorf("%s: %q is a whiteout marker file", d.fsys.Name(), p)
			case strings.Contains(name, ".opq"):
				t.Errorf("%s: %q is an opaque marker file", d.fsys.Name(), p)
			case entry.Type()&(fs.ModeDevice|fs.ModeCharDevice) != 0:
				t.Errorf(
					"%s: %q is a device, which is how the reference spells a whiteout",
					d.fsys.Name(),
					p,
				)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("walk %s: %v", d.fsys.Name(), err)
	}
}
