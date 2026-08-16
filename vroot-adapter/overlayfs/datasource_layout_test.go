package overlayfs

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
)

func names(t *testing.T, fsys vroot.Fs[vroot.File], dir string) []string {
	t.Helper()
	entries, err := vroot.ReadDir(fsys, dir)
	if err != nil {
		t.Fatalf("read dir %q: %v", dir, err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	slices.Sort(got)
	return got
}

// TestDataSourcePaths checks how the two views address a name.
func TestDataSourcePaths(t *testing.T) {
	plain := NewDataSource[vroot.File](memfs.New("mem"))
	defer plain.Close()

	canonical, err := NewDataSourceCanonical[vroot.File](memfs.New("mem"))
	if err != nil {
		t.Fatalf("NewDataSourceCanonical: %v", err)
	}
	defer canonical.Close()

	for _, name := range []string{"a", "a/b"} {
		if want := filepath.FromSlash(name); plain.ContentPath(name) != want {
			t.Errorf("plain ContentPath(%q) = %q, want %q", name, plain.ContentPath(name), want)
		}
		if want := filepath.Join(dirMerged, name); canonical.ContentPath(name) != want {
			t.Errorf(
				"canonical ContentPath(%q) = %q, want %q",
				name,
				canonical.ContentPath(name),
				want,
			)
		}
		if want := filepath.Join(dirWork, name); canonical.StagingPath(name) != want {
			t.Errorf(
				"canonical StagingPath(%q) = %q, want %q",
				name,
				canonical.StagingPath(name),
				want,
			)
		}
		// A plain layer has no staging tree, and says so with a path that addresses
		// nothing rather than by pointing at a "work/" it does not hold.
		if got := plain.StagingPath(name); got != "" {
			t.Errorf("plain StagingPath(%q) = %q, want %q", name, got, "")
		}
	}
}

// TestDataSourceSweepWork checks that the sweep empties the staging directory
// and touches nothing else.
func TestDataSourceSweepWork(t *testing.T) {
	fsys := memfs.New("mem")
	d, err := NewDataSourceCanonical[vroot.File](fsys)
	if err != nil {
		t.Fatalf("NewDataSourceCanonical: %v", err)
	}
	defer d.Close()

	create(t, fsys, d.StagingPath("tmp-0"))
	if err := fsys.Mkdir(d.StagingPath("tmp-1"), 0o777); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	create(t, fsys, d.StagingPath(filepath.Join("tmp-1", "inner")))
	create(t, fsys, d.ContentPath("keep"))

	if err := d.sweepWork(); err != nil {
		t.Fatalf("sweepWork: %v", err)
	}
	if got := names(t, fsys, dirWork); len(got) != 0 {
		t.Errorf("%q after the sweep holds %q, want nothing", dirWork, got)
	}
	if got := names(t, fsys, dirMerged); !slices.Equal(got, []string{"keep"}) {
		t.Errorf("%q after the sweep holds %q, want %q", dirMerged, got, []string{"keep"})
	}
	if !exists(t, fsys, storeName) {
		t.Errorf("%q removed by the sweep", storeName)
	}

	if err := d.sweepWork(); err != nil {
		t.Fatalf("sweepWork over an empty staging directory: %v", err)
	}
	if err := fsys.Remove(dirWork); err != nil {
		t.Fatalf("remove %q: %v", dirWork, err)
	}
	if err := d.sweepWork(); err != nil {
		t.Fatalf("sweepWork with no staging directory: %v", err)
	}
}
