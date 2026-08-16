package overlayfs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"golang.org/x/sync/errgroup"
)

// TestFsReadShadowing checks that a name is read from the shallowest layer
// showing it, top before lowers[0] before lowers[1].
func TestFsReadShadowing(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, lower0, lower1 := newOverlay(t, mk)

		put(t, top, "all", "from-top", 0o666)
		put(t, lower0, "all", "from-lower0", 0o666)
		put(t, lower1, "all", "from-lower1", 0o666)
		put(t, lower0, "dir/both", "from-lower0", 0o666)
		put(t, lower1, "dir/both", "from-lower1", 0o666)
		put(t, lower1, "dir/deep/only", "from-lower1", 0o666)

		assertRead(t, f, "all", "from-top")
		assertRead(t, f, "dir/both", "from-lower0")
		assertRead(t, f, filepath.FromSlash("dir/deep/only"), "from-lower1")

		_, err := f.Open("missing")
		assertErrIs(t, `Open("missing")`, err, fs.ErrNotExist)
	})
}

// TestFsStatLstat checks that Stat follows the final component and Lstat does
// not, while both follow the symlinks on the way there.
func TestFsStatLstat(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, lower0, _ := newOverlay(t, mk)

		put(t, lower0, "dir/target", "0123456789", 0o666)
		link(t, top, "dir", "dirlink")
		link(t, top, "target", "dir/inner")

		info, err := f.Stat(filepath.FromSlash("dirlink/inner"))
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if info.Mode()&fs.ModeSymlink != 0 || info.Size() != 10 {
			t.Errorf(
				"Stat through two links = %v size %d, want the target file",
				info.Mode(),
				info.Size(),
			)
		}

		info, err = f.Lstat(filepath.FromSlash("dirlink/inner"))
		if err != nil {
			t.Fatalf("Lstat: %v", err)
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			t.Errorf("Lstat of a symlink = %v, want a symlink", info.Mode())
		}

		target, err := f.ReadLink("dirlink")
		if err != nil {
			t.Fatalf("ReadLink: %v", err)
		}
		if target != "dir" {
			t.Errorf("ReadLink(dirlink) = %q, want %q", target, "dir")
		}
		if _, err := f.ReadLink(filepath.FromSlash("dir/target")); err == nil {
			t.Error("ReadLink of a regular file succeeded")
		}
	})
}

// TestFsCrossLayerSymlink checks that resolution asks the merged view at every
// step: a link the top holds resolves into content only a lower shows.
func TestFsCrossLayerSymlink(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, lower0, lower1 := newOverlay(t, mk)

		put(t, lower1, "deep/data.txt", "from-lower1", 0o666)
		link(t, lower0, "../deep", "mid/hop")
		link(t, top, "mid/hop", "entry")

		assertRead(t, f, filepath.FromSlash("entry/data.txt"), "from-lower1")
	})
}

// TestFsEscapes checks that nothing addresses anything above the overlay root,
// whether it is spelled with "..", as an absolute path, or as a symlink.
func TestFsEscapes(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, _, _ := newOverlay(t, mk)

		put(t, top, "sub/inside.txt", "in", 0o666)
		link(t, top, "../outside", "escapelink")
		link(t, top, "../../outside", "sub/escapelink")

		for _, name := range []string{
			"..",
			filepath.FromSlash("../sibling"),
			filepath.FromSlash("sub/../.."),
			string(filepath.Separator) + "abs",
			"escapelink",
			filepath.FromSlash("sub/escapelink"),
		} {
			_, err := f.Open(name)
			assertErrIs(t, "Open("+name+")", err, vroot.ErrPathEscapes)
			_, err = f.Stat(name)
			assertErrIs(t, "Stat("+name+")", err, vroot.ErrPathEscapes)
			_, err = f.OpenRoot(name)
			assertErrIs(t, "OpenRoot("+name+")", err, vroot.ErrPathEscapes)
		}
	})
}

// TestFsMergedDirHandle checks the listing a directory handle serves: every
// layer's entries merged and sorted, read in chunks by a cursor that does not
// hand the same entry out twice and is rewound only by a seek to offset zero.
func TestFsMergedDirHandle(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, lower0, lower1 := newOverlay(t, mk)

		put(t, top, "dir/a", "a", 0o666)
		put(t, top, "dir/shared", "from-top", 0o666)
		put(t, lower0, "dir/b", "b", 0o666)
		put(t, lower0, "dir/shared", "from-lower0", 0o666)
		put(t, lower1, "dir/c", "c", 0o666)
		want := []string{"a", "b", "c", "shared"}

		d, err := f.Open("dir")
		if err != nil {
			t.Fatalf("open dir: %v", err)
		}
		defer func() { _ = d.Close() }()

		if name := d.Name(); name != "dir" {
			t.Errorf("Name() = %q, want %q", name, "dir")
		}

		var chunked []string
		for {
			entries, err := d.ReadDir(2)
			for _, e := range entries {
				chunked = append(chunked, e.Name())
			}
			if err == io.EOF || len(entries) == 0 {
				break
			}
			if err != nil {
				t.Fatalf("ReadDir(2): %v", err)
			}
			if len(entries) > 2 {
				t.Fatalf("ReadDir(2) returned %d entries", len(entries))
			}
		}
		if !slices.Equal(chunked, want) {
			t.Errorf("chunked ReadDir = %q, want %q", chunked, want)
		}

		_, err = d.Seek(1, io.SeekStart)
		assertErrIs(t, "Seek(1, io.SeekStart)", err, fs.ErrInvalid)

		if _, err := d.Seek(0, io.SeekStart); err != nil {
			t.Fatalf("Seek: %v", err)
		}
		infos, err := d.Readdir(-1)
		if err != nil {
			t.Fatalf("Readdir: %v", err)
		}
		got := make([]string, len(infos))
		for i, info := range infos {
			got[i] = info.Name()
		}
		if !slices.Equal(got, want) {
			t.Errorf("Readdir after rewind = %q, want %q", got, want)
		}

		if _, err := d.Seek(0, io.SeekStart); err != nil {
			t.Fatalf("Seek: %v", err)
		}
		names, err := d.Readdirnames(-1)
		if err != nil {
			t.Fatalf("Readdirnames: %v", err)
		}
		if !slices.Equal(names, want) {
			t.Errorf("Readdirnames after rewind = %q, want %q", names, want)
		}

		if infos, err := d.Readdir(-1); err != nil || len(infos) != 0 {
			t.Errorf("Readdir past the end = (%d entries, %v), want (0, nil)", len(infos), err)
		}
		if _, err := d.ReadDir(1); err != io.EOF {
			t.Errorf("ReadDir(1) past the end = %v, want io.EOF", err)
		}

		if _, err := d.Read(make([]byte, 1)); err == nil {
			t.Error("Read on a directory handle succeeded")
		}
	})
}

// TestFsMergedDirRoot checks that the overlay root lists like any other merged
// directory, through the handle Open(".") returns.
func TestFsMergedDirRoot(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, lower0, lower1 := newOverlay(t, mk)

		put(t, top, "from-top", "x", 0o666)
		put(t, lower0, "from-lower0", "x", 0o666)
		put(t, lower1, "from-lower1", "x", 0o666)

		entries, err := vroot.ReadDir[vroot.File](f, ".")
		if err != nil {
			t.Fatalf("ReadDir: %v", err)
		}
		got := make([]string, len(entries))
		for i, e := range entries {
			got[i] = e.Name()
		}
		want := []string{"from-lower0", "from-lower1", "from-top"}
		if !slices.Equal(got, want) {
			t.Errorf("root listing = %q, want %q", got, want)
		}
	})
}

// TestFsLowerFilesReadOnly checks that a handle onto a lower refuses every write
// while the same file in the top passes its layer's handle through.
func TestFsLowerFilesReadOnly(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, lower0, _ := newOverlay(t, mk)

		put(t, top, "mine", "top", 0o666)
		put(t, lower0, "theirs", "lower", 0o666)

		lower, err := f.Open("theirs")
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = lower.Close() }()

		if _, err := lower.Write([]byte("x")); err == nil {
			t.Error("Write through a lower-backed handle succeeded")
		}
		if _, err := lower.WriteAt([]byte("x"), 0); err == nil {
			t.Error("WriteAt through a lower-backed handle succeeded")
		}
		if err := lower.Truncate(0); err == nil {
			t.Error("Truncate through a lower-backed handle succeeded")
		}
		if err := lower.Chmod(0o600); err == nil {
			t.Error("Chmod through a lower-backed handle succeeded")
		}
		if name := lower.Name(); name != "theirs" {
			t.Errorf("Name() = %q, want %q", name, "theirs")
		}

		topFile, err := f.Open("mine")
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		defer func() { _ = topFile.Close() }()
		if _, err := io.ReadAll(topFile); err != nil {
			t.Errorf("read through a top-backed handle: %v", err)
		}
	})
}

// TestFsOpenRoot checks the sub-overlay: it reads the same merged view confined
// to one directory, and confines escapes to that directory rather than to the
// overlay it was opened from.
func TestFsOpenRoot(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, lower0, lower1 := newOverlay(t, mk)

		put(t, top, "sub/from-top", "from-top", 0o666)
		put(t, lower0, "sub/from-lower0", "from-lower0", 0o666)
		put(t, lower1, "sub/deep/from-lower1", "from-lower1", 0o666)
		put(t, top, "outside", "outside", 0o666)
		link(t, top, "../outside", "sub/climb")

		sub, err := f.OpenRoot("sub")
		if err != nil {
			t.Fatalf("OpenRoot: %v", err)
		}
		defer func() { _ = sub.Close() }()

		assertRead(t, sub, "from-top", "from-top")
		assertRead(t, sub, "from-lower0", "from-lower0")
		assertRead(t, sub, filepath.FromSlash("deep/from-lower1"), "from-lower1")

		entries, err := vroot.ReadDir[vroot.File](sub, ".")
		if err != nil {
			t.Fatalf("ReadDir: %v", err)
		}
		got := make([]string, len(entries))
		for i, e := range entries {
			got[i] = e.Name()
		}
		want := []string{"climb", "deep", "from-lower0", "from-top"}
		if !slices.Equal(got, want) {
			t.Errorf("sub-overlay root listing = %q, want %q", got, want)
		}

		// The link resolves inside the overlay the sub-root was opened from and
		// still must not resolve inside the sub-root.
		assertRead(t, f, filepath.FromSlash("sub/climb"), "outside")
		_, err = sub.Open("climb")
		assertErrIs(t, `sub.Open("climb")`, err, vroot.ErrPathEscapes)
		_, err = sub.Open("..")
		assertErrIs(t, `sub.Open("..")`, err, vroot.ErrPathEscapes)
		_, err = sub.Open(filepath.FromSlash("../outside"))
		assertErrIs(t, `sub.Open("../outside")`, err, vroot.ErrPathEscapes)

		if _, err := f.OpenRoot("outside"); err == nil {
			t.Error("OpenRoot on a regular file succeeded")
		}
		if _, err := f.OpenRoot("missing"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("OpenRoot on a missing name -> %v, want an error matching fs.ErrNotExist", err)
		}
		if sub.Name() == f.Name() {
			t.Errorf("sub-overlay Name() = %q, same as the overlay it was opened from", sub.Name())
		}
	})
}

// TestFsOpenHandleCount checks the accounting the removal gate reads: a handle
// counts from the open that produced it until the close that ends it, once,
// under the name the open resolved to.
func TestFsOpenHandleCount(t *testing.T) {
	f, top, _, _ := newOverlay(t, memfsMaker)
	put(t, top, "dir/data", "x", 0o666)
	link(t, top, "dir/data", "alias")

	count := func(name string) int { return f.handles.count(name) }

	first, err := f.Open(filepath.FromSlash("dir/data"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// The alias resolves onto the same name, so both handles count there.
	second, err := f.Open("alias")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got := count("dir/data"); got != 2 {
		t.Errorf("open handles on dir/data = %d, want 2", got)
	}
	if got := count("alias"); got != 0 {
		t.Errorf("open handles on alias = %d, want 0", got)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if got := count("dir/data"); got != 1 {
		t.Errorf("open handles after closing one twice = %d, want 1", got)
	}

	dir, err := f.Open("dir")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got := count("dir"); got != 1 {
		t.Errorf("open handles on the directory = %d, want 1", got)
	}

	for _, file := range []vroot.File{second, dir} {
		if err := file.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	}
	for _, name := range []string{"dir", "dir/data"} {
		if got := count(name); got != 0 {
			t.Errorf("open handles on %q after every close = %d, want 0", name, got)
		}
	}
}

// TestFsConcurrentReads runs the whole read path at once — resolution, opens,
// merged listings, sub-overlays — over the state and the handle count they all
// share, and checks that every handle was counted back in afterwards.
func TestFsConcurrentReads(t *testing.T) {
	f, top, lower0, lower1 := newOverlay(t, memfsMaker)
	put(t, top, "dir/from-top", "from-top", 0o666)
	put(t, lower0, "dir/from-lower0", "from-lower0", 0o666)
	put(t, lower1, "dir/from-lower1", "from-lower1", 0o666)
	link(t, top, "dir", "dirlink")

	names := []string{
		filepath.FromSlash("dir/from-top"),
		filepath.FromSlash("dirlink/from-lower0"),
		filepath.FromSlash("dirlink/from-lower1"),
	}

	var g errgroup.Group
	for range 8 {
		g.Go(func() error {
			for range 32 {
				for _, name := range names {
					file, err := f.Open(name)
					if err != nil {
						return fmt.Errorf("open %q: %w", name, err)
					}
					_, err = io.ReadAll(file)
					if err := errors.Join(err, file.Close()); err != nil {
						return fmt.Errorf("read %q: %w", name, err)
					}
				}
				dir, err := f.Open("dirlink")
				if err != nil {
					return err
				}
				_, err = dir.ReadDir(-1)
				if err := errors.Join(err, dir.Close()); err != nil {
					return err
				}
				sub, err := f.OpenRoot("dirlink")
				if err != nil {
					return err
				}
				_, err = sub.Stat("from-lower1")
				if err := errors.Join(err, sub.Close()); err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("concurrent reads: %v", err)
	}

	for _, name := range []string{"dir", "dir/from-top", "dir/from-lower0", "dir/from-lower1"} {
		if got := f.handles.count(name); got != 0 {
			t.Errorf("open handles on %q after every close = %d, want 0", name, got)
		}
	}
}
