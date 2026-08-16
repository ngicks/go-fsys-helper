package overlayfs

import (
	"errors"
	"io/fs"
	"os"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
	"github.com/ngicks/go-fsys-helper/vroot/osfs"
)

type backing struct {
	name string
	// open hands out a fresh handle onto one and the same backing every time it
	// is called, so a data source built after another was closed finds the layout
	// that one left behind.
	open func(t *testing.T) func(t *testing.T) vroot.Fs[vroot.File]
}

// backings lists the fsys implementations a data source must work over: memfs,
// which is type-erased and in memory, and osfs, which has a concrete file type
// and a real directory under it.
func backings() []backing {
	return []backing{
		{"memfs", func(t *testing.T) func(*testing.T) vroot.Fs[vroot.File] {
			fsys := memfs.New("mem")
			return func(*testing.T) vroot.Fs[vroot.File] { return fsys }
		}},
		{"osfs", func(t *testing.T) func(*testing.T) vroot.Fs[vroot.File] {
			dir := t.TempDir()
			return func(t *testing.T) vroot.Fs[vroot.File] { return newOsfs(t, dir) }
		}},
	}
}

func newOsfs(t *testing.T, dir string) vroot.Fs[vroot.File] {
	t.Helper()
	fsys, err := osfs.NewFs(dir)
	if err != nil {
		t.Fatalf("osfs.NewFs: %v", err)
	}
	return vroot.Widen[*osfs.File](fsys)
}

func exists(t *testing.T, fsys vroot.Fs[vroot.File], name string) bool {
	t.Helper()
	_, err := fsys.Stat(name)
	if err == nil {
		return true
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("stat %q: %v", name, err)
	}
	return false
}

func assertDir(t *testing.T, fsys vroot.Fs[vroot.File], name string) {
	t.Helper()
	info, err := fsys.Stat(name)
	if err != nil {
		t.Fatalf("stat %q: %v", name, err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", name)
	}
}

func create(t *testing.T, fsys vroot.Fs[vroot.File], name string) {
	t.Helper()
	f, err := fsys.Create(name)
	if err != nil {
		t.Fatalf("create %q: %v", name, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %q: %v", name, err)
	}
}

// TestNewDataSource checks the plain form: the fs is left exactly as it is, and
// nothing of the canonical layout appears in it.
func TestNewDataSource(t *testing.T) {
	fsys := memfs.New("mem")
	d := NewDataSource[vroot.File](fsys)

	if d.canonical {
		t.Error("plain data source reports itself canonical")
	}
	if d.store != nil {
		t.Errorf("plain data source carries a store %T", d.store)
	}
	for _, name := range []string{dirMerged, dirWork, storeName} {
		if exists(t, fsys, name) {
			t.Errorf("%q created for a plain data source", name)
		}
	}
	if err := d.sweepWork(); err != nil {
		t.Errorf("sweepWork on a plain data source: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestNewDataSourceCanonical checks that a fresh fs gets the whole layout, and
// that a data source built over that layout again reads the masking state the
// closed one wrote.
func TestNewDataSourceCanonical(t *testing.T) {
	for _, b := range backings() {
		t.Run(b.name, func(t *testing.T) {
			open := b.open(t)
			fsys := open(t)

			d, err := NewDataSourceCanonical(fsys)
			if err != nil {
				t.Fatalf("NewDataSourceCanonical: %v", err)
			}
			if !d.canonical {
				t.Error("canonical data source reports itself plain")
			}
			if d.readOnly {
				t.Error("store opened read-only over a writable backend")
			}
			assertDir(t, fsys, dirMerged)
			assertDir(t, fsys, dirWork)
			if !exists(t, fsys, storeName) {
				t.Fatalf("%q not created", storeName)
			}

			if err := d.store.SetWhiteout("gone"); err != nil {
				t.Fatalf("SetWhiteout: %v", err)
			}
			if err := d.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			again, err := NewDataSourceCanonical(open(t))
			if err != nil {
				t.Fatalf("NewDataSourceCanonical over the same layout: %v", err)
			}
			defer again.Close()

			whiteouts, _, err := again.store.Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !slices.Equal(whiteouts, []string{"gone"}) {
				t.Errorf("whiteouts after reopen = %q, want %q", whiteouts, []string{"gone"})
			}
		})
	}
}

// TestNewDataSourceCanonicalNotADirectory checks that a name the layout needs,
// taken by a file, is reported instead of worked around.
func TestNewDataSourceCanonicalNotADirectory(t *testing.T) {
	for _, name := range []string{dirMerged, dirWork} {
		t.Run(name, func(t *testing.T) {
			fsys := memfs.New("mem")
			create(t, fsys, name)

			d, err := NewDataSourceCanonical[vroot.File](fsys)
			if err == nil {
				_ = d.Close()
				t.Fatalf("NewDataSourceCanonical accepted a file named %q", name)
			}
			if !errors.Is(err, ErrNotCanonical) {
				t.Errorf("err = %v, want one matching ErrNotCanonical", err)
			}
		})
	}
}

// TestNewDataSourceCanonicalReadOnly checks the fallback a backend that refuses
// writes gets: the store still loads what the layer was left with, and the data
// source records that it can only be a lower.
func TestNewDataSourceCanonicalReadOnly(t *testing.T) {
	fsys := memfs.New("mem")

	written, err := NewDataSourceCanonical[vroot.File](fsys)
	if err != nil {
		t.Fatalf("NewDataSourceCanonical: %v", err)
	}
	if err := written.store.SetWhiteout("gone"); err != nil {
		t.Fatalf("SetWhiteout: %v", err)
	}
	if err := written.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	d, err := NewDataSourceCanonical[*vroot.ReadOnlyFile](vroot.NewReadOnlyFs[vroot.File](fsys))
	if err != nil {
		t.Fatalf("NewDataSourceCanonical over a read-only fs: %v", err)
	}
	defer d.Close()

	if !d.readOnly {
		t.Error("store over a read-only fs reports itself writable")
	}
	whiteouts, _, err := d.store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !slices.Equal(whiteouts, []string{"gone"}) {
		t.Errorf("whiteouts = %q, want %q", whiteouts, []string{"gone"})
	}
	if err := d.store.SetWhiteout("more"); err == nil {
		t.Error("SetWhiteout through a read-only store succeeded")
	}
}

// TestNewDataSourceCanonicalReadOnlyWithoutStore checks that a read-only layout
// without a database is refused: nothing here can create one, and the layer
// would silently lose the masking it was distributed with.
func TestNewDataSourceCanonicalReadOnlyWithoutStore(t *testing.T) {
	fsys := memfs.New("mem")
	for _, name := range []string{dirMerged, dirWork} {
		if err := fsys.Mkdir(name, 0o777); err != nil {
			t.Fatalf("mkdir %q: %v", name, err)
		}
	}

	d, err := NewDataSourceCanonical[*vroot.ReadOnlyFile](vroot.NewReadOnlyFs[vroot.File](fsys))
	if err == nil {
		_ = d.Close()
		t.Fatal("NewDataSourceCanonical accepted a read-only layout with no store")
	}
}

func lockable(t *testing.T, fsys vroot.Fs[vroot.File]) bool {
	t.Helper()
	f, err := fsys.OpenFile("lock-probe", os.O_RDWR|os.O_CREATE, 0o666)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer func() {
		_ = f.Close()
		_ = fsys.Remove("lock-probe")
	}()
	_, ok := f.(vroot.Locker)
	return ok
}

// TestDataSourceCloseReleasesStore checks that Close gives the database back: a
// data source built afterwards over the same directory gets in. Only that
// direction is asserted — one built while the first still held the store would
// wait for the lock, not fail.
func TestDataSourceCloseReleasesStore(t *testing.T) {
	dir := t.TempDir()
	first := newOsfs(t, dir)
	if !lockable(t, first) {
		t.Skipf("osfs files do not implement vroot.Locker on %s", runtime.GOOS)
	}

	d, err := NewDataSourceCanonical(first)
	if err != nil {
		t.Fatalf("NewDataSourceCanonical: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second := newOsfs(t, dir)
	opened := make(chan error, 1)
	go func() {
		again, err := NewDataSourceCanonical(second)
		if err == nil {
			err = again.Close()
		}
		opened <- err
	}()
	select {
	case err := <-opened:
		if err != nil {
			t.Fatalf("data source after the first closed: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("data source still blocked after the first closed")
	}
}
