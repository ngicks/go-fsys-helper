package overlayfs

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
)

func canonicalSource(t *testing.T, fsys vroot.Fs[vroot.File]) *DataSource {
	t.Helper()
	d, err := NewDataSourceCanonical(fsys)
	if err != nil {
		t.Fatalf("NewDataSourceCanonical(%q): %v", fsys.Name(), err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// threeLayers is the stack the tests share: a canonical top over a canonical
// lower over a plain one, so both a mutable and a frozen store are in play.
func threeLayers(t *testing.T) (top, lower0, lower1 *DataSource) {
	t.Helper()
	top = canonicalSource(t, memfs.New("top"))
	lower0 = canonicalSource(t, memfs.New("lower0"))
	lower1 = NewDataSource[vroot.File](memfs.New("lower1"))
	t.Cleanup(func() { _ = lower1.Close() })
	return top, lower0, lower1
}

func newState(t *testing.T, top *DataSource, lowers ...*DataSource) *overlayState {
	t.Helper()
	s, err := newOverlayState(top, lowers)
	if err != nil {
		t.Fatalf("newOverlayState: %v", err)
	}
	return s
}

// seed creates names in the layer's content tree, parents included; a name
// ending in "/" becomes a directory, anything else an empty file.
func seed(t *testing.T, d *DataSource, names ...string) {
	t.Helper()
	for _, name := range names {
		path := d.ContentPath(strings.TrimSuffix(name, "/"))
		dir := path
		if !strings.HasSuffix(name, "/") {
			dir = filepath.Dir(path)
		}
		if err := d.fsys.MkdirAll(dir, 0o777); err != nil {
			t.Fatalf("mkdirall %q: %v", dir, err)
		}
		if !strings.HasSuffix(name, "/") {
			create(t, d.fsys, path)
		}
	}
}

func mark(t *testing.T, op string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", op, err)
	}
}

func mutate(t *testing.T, s *overlayState, op string, fn func() error) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	mark(t, op, fn())
}

func assertLayer(t *testing.T, s *overlayState, name string, want int) {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()
	res, err := s.lookup(name)
	if err != nil {
		t.Errorf("lookup %q: %v, want layer %d", name, err, want)
		return
	}
	if res.layer != want {
		t.Errorf("lookup %q -> layer %d, want %d", name, res.layer, want)
	}
	if res.src != s.layers[want] || res.path != s.layerPath(want, cleanName(name)) {
		t.Errorf("lookup %q -> %q in %q, want the layer %d content path",
			name, res.path, res.src.fsys.Name(), want)
	}
}

func assertLookupErr(t *testing.T, s *overlayState, name string, want error) {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()
	res, err := s.lookup(name)
	if !errors.Is(err, want) {
		t.Errorf(
			"lookup %q -> (layer %d, %v), want an error matching %v",
			name,
			res.layer,
			err,
			want,
		)
	}
}

func assertEntries(t *testing.T, s *overlayState, name string, want ...string) {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries, err := s.readDir(name)
	if err != nil {
		t.Fatalf("readDir %q: %v", name, err)
	}
	got := make([]string, len(entries))
	for i, e := range entries {
		got[i] = e.Name()
	}
	if !slices.Equal(got, want) {
		t.Errorf("readDir %q -> %q, want %q", name, got, want)
	}
}

// visibility renders where probes resolve, so two states can be compared entry
// by entry.
func visibility(t *testing.T, s *overlayState, probes ...string) []string {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(probes))
	for i, probe := range probes {
		res, err := s.lookup(probe)
		switch {
		case err == nil:
			out[i] = fmt.Sprintf("%s=layer%d", probe, res.layer)
		case errors.Is(err, fs.ErrNotExist):
			out[i] = probe + "=missing"
		default:
			out[i] = fmt.Sprintf("%s=%v", probe, err)
		}
	}
	return out
}

// TestNewOverlayStateNotCanonical checks that a top with no store of its own is
// refused, the masking the overlay is about to write having nowhere to go.
func TestNewOverlayStateNotCanonical(t *testing.T) {
	plain := NewDataSource[vroot.File](memfs.New("plain"))
	defer plain.Close()

	s, err := newOverlayState(plain, nil)
	if !errors.Is(err, ErrNotCanonical) {
		t.Errorf(
			"newOverlayState(plain, nil) -> (%v, %v), want an error matching ErrNotCanonical",
			s,
			err,
		)
	}
}

// TestOverlayStateWhiteoutTop checks that a whiteout the top records hides the
// name in every lower, and that clearing it brings the lowers back.
func TestOverlayStateWhiteoutTop(t *testing.T) {
	top, lower0, lower1 := threeLayers(t)
	seed(t, top, "kept")
	seed(t, lower0, "gone", "kept")
	seed(t, lower1, "gone")

	s := newState(t, top, lower0, lower1)
	assertLayer(t, s, "gone", 1)

	mutate(t, s, "setWhiteout", func() error { return s.setWhiteout("gone") })
	assertLookupErr(t, s, "gone", fs.ErrNotExist)
	assertEntries(t, s, "", "kept")

	s.mu.RLock()
	whiteout, _ := s.marks(0, "gone")
	s.mu.RUnlock()
	if !whiteout {
		t.Error("marks(0, gone) reports no whiteout after setWhiteout")
	}

	mutate(t, s, "clearWhiteout", func() error { return s.clearWhiteout("gone") })
	assertLayer(t, s, "gone", 1)
	assertEntries(t, s, "", "gone", "kept")
}

// TestOverlayStateWhiteoutLower checks per-layer provenance: a whiteout the
// canonical lower recorded masks only the layers below it, leaves the top's own
// entry of that name alone, and does not even hide the layer that recorded it.
func TestOverlayStateWhiteoutLower(t *testing.T) {
	top, lower0, lower1 := threeLayers(t)
	seed(t, top, "shadowed")
	seed(t, lower0, "shadowed", "kept-here")
	seed(t, lower1, "shadowed", "masked", "kept-here")

	mark(t, "SetWhiteout", lower0.store.SetWhiteout("shadowed"))
	mark(t, "SetWhiteout", lower0.store.SetWhiteout("masked"))
	mark(t, "SetWhiteout", lower0.store.SetWhiteout("kept-here"))

	s := newState(t, top, lower0, lower1)

	assertLayer(t, s, "shadowed", 0)
	assertLayer(t, s, "kept-here", 1)
	assertLookupErr(t, s, "masked", fs.ErrNotExist)
	assertEntries(t, s, "", "kept-here", "shadowed")
}

// TestOverlayStateWhiteoutAncestor checks that a whiteout on a directory takes
// the whole subtree the lowers show under it with it.
func TestOverlayStateWhiteoutAncestor(t *testing.T) {
	top, lower0, lower1 := threeLayers(t)
	seed(t, lower0, "dir/file", "dir/sub/deep")
	seed(t, lower1, "dir/other")

	s := newState(t, top, lower0, lower1)
	assertLayer(t, s, "dir/sub/deep", 1)

	mutate(t, s, "setWhiteout", func() error { return s.setWhiteout("dir") })
	for _, name := range []string{"dir", "dir/file", "dir/sub", "dir/sub/deep", "dir/other"} {
		assertLookupErr(t, s, name, fs.ErrNotExist)
	}
}

// TestOverlayStateWhiteoutDropsSubtree checks that a whiteout forgets the marks
// recorded under it and its own opaque mark, in memory as in the store.
func TestOverlayStateWhiteoutDropsSubtree(t *testing.T) {
	fsys := memfs.New("top")
	top := canonicalSource(t, fsys)
	s := newState(t, top)

	for _, op := range []struct {
		name string
		fn   func(string) error
	}{
		{"dir", s.setOpaque},
		{"dir/kept", s.setWhiteout},
		{"dir/sub", s.setOpaque},
		{"dirt", s.setWhiteout},
	} {
		mutate(t, s, "seed mark", func() error { return op.fn(op.name) })
	}

	mutate(t, s, "setWhiteout", func() error { return s.setWhiteout("dir") })

	s.mu.RLock()
	defer s.mu.RUnlock()
	whiteout, opaque := s.marks(0, "dir")
	if !whiteout || opaque {
		t.Errorf("marks(0, dir) = (%v, %v), want (true, false)", whiteout, opaque)
	}
	for _, name := range []string{"dir/kept", "dir/sub"} {
		if whiteout, opaque := s.marks(0, name); whiteout || opaque {
			t.Errorf("marks(0, %q) = (%v, %v), want both false", name, whiteout, opaque)
		}
	}
	// A sibling whose name merely starts with the whited-out one is not under it.
	if whiteout, _ := s.marks(0, "dirt"); !whiteout {
		t.Error("marks(0, dirt) lost the whiteout of a name outside the subtree")
	}
}

// TestOverlayStateOpaque checks that an opaque mark cuts the layers below the
// one that recorded it out of a directory, and only those.
func TestOverlayStateOpaque(t *testing.T) {
	top, lower0, lower1 := threeLayers(t)
	seed(t, top, "dir/from-top")
	seed(t, lower0, "dir/from-lower0")
	seed(t, lower1, "dir/from-lower1")

	s := newState(t, top, lower0, lower1)
	assertEntries(t, s, "dir", "from-lower0", "from-lower1", "from-top")

	mark(t, "SetOpaque", lower0.store.SetOpaque("dir"))
	s = newState(t, top, lower0, lower1)
	assertEntries(t, s, "dir", "from-lower0", "from-top")
	assertLayer(t, s, "dir/from-lower0", 1)
	assertLookupErr(t, s, "dir/from-lower1", fs.ErrNotExist)

	mutate(t, s, "setOpaque", func() error { return s.setOpaque("dir") })
	assertEntries(t, s, "dir", "from-top")
	assertLayer(t, s, "dir", 0)
	assertLookupErr(t, s, "dir/from-lower0", fs.ErrNotExist)

	mutate(t, s, "clearOpaque", func() error { return s.clearOpaque("dir") })
	assertEntries(t, s, "dir", "from-lower0", "from-top")
}

// TestOverlayStateOpaqueSubtree checks that an opaque directory hides the whole
// subtree the layers below it show, not just their direct children.
func TestOverlayStateOpaqueSubtree(t *testing.T) {
	top, lower0, lower1 := threeLayers(t)
	seed(t, top, "dir/")
	seed(t, lower0, "dir/sub/deep")

	s := newState(t, top, lower0, lower1)
	assertLayer(t, s, "dir/sub/deep", 1)

	mutate(t, s, "setOpaque", func() error { return s.setOpaque("dir") })
	assertLookupErr(t, s, "dir/sub", fs.ErrNotExist)
	assertLookupErr(t, s, "dir/sub/deep", fs.ErrNotExist)
}

// TestOverlayStateRoot checks the root, which every ancestor walk ends at: it
// merges like any other directory, an opaque mark on it cuts the lowers out,
// and a whiteout on it forgets every mark the top recorded.
func TestOverlayStateRoot(t *testing.T) {
	top, lower0, lower1 := threeLayers(t)
	seed(t, top, "from-top")
	seed(t, lower0, "from-lower0")
	seed(t, lower1, "from-lower1")

	s := newState(t, top, lower0, lower1)
	assertLayer(t, s, "", 0)
	assertLayer(t, s, ".", 0)
	assertEntries(t, s, "", "from-lower0", "from-lower1", "from-top")

	mutate(t, s, "setWhiteout", func() error { return s.setWhiteout("from-lower0") })
	mutate(t, s, "setOpaque", func() error { return s.setOpaque("") })
	assertEntries(t, s, "", "from-top")
	assertLookupErr(t, s, "from-lower1", fs.ErrNotExist)

	mutate(t, s, "setWhiteout", func() error { return s.setWhiteout("") })
	s.mu.RLock()
	defer s.mu.RUnlock()
	if whiteout, opaque := s.marks(0, ""); !whiteout || opaque {
		t.Errorf("marks(0, root) = (%v, %v), want (true, false)", whiteout, opaque)
	}
	if whiteout, _ := s.marks(0, "from-lower0"); whiteout {
		t.Error("a whiteout on the root kept a mark recorded under it")
	}
}

// TestOverlayStateMutatorsWriteThrough checks that every mutator has reached
// the top's store by the time it returns: the store is reopened on its own,
// after the data source that wrote through it is closed.
func TestOverlayStateMutatorsWriteThrough(t *testing.T) {
	fsys := memfs.New("top")
	top := canonicalSource(t, fsys)
	s := newState(t, top)

	mutate(t, s, "setWhiteout", func() error { return s.setWhiteout("gone") })
	mutate(t, s, "setWhiteout", func() error { return s.setWhiteout("undone") })
	mutate(t, s, "clearWhiteout", func() error { return s.clearWhiteout("undone") })
	mutate(t, s, "setOpaque", func() error { return s.setOpaque("dir") })
	mutate(t, s, "setOpaque", func() error { return s.setOpaque("dir/undone") })
	mutate(t, s, "clearOpaque", func() error { return s.clearOpaque("dir/undone") })

	if err := top.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	store, err := NewMetadataStoreSQLite(fsys, storeName)
	if err != nil {
		t.Fatalf("NewMetadataStoreSQLite: %v", err)
	}
	defer store.Close()

	whiteouts, opaques, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !slices.Equal(whiteouts, []string{"gone"}) {
		t.Errorf("whiteouts = %q, want %q", whiteouts, []string{"gone"})
	}
	if !slices.Equal(opaques, []string{"dir"}) {
		t.Errorf("opaques = %q, want %q", opaques, []string{"dir"})
	}
}

// TestOverlayStateSeedFromStore checks that state torn down and built again
// over the same layers answers exactly as it did, the stores having kept every
// mask both the top and a lower recorded.
func TestOverlayStateSeedFromStore(t *testing.T) {
	topFsys := memfs.New("top")
	lower0Fsys := memfs.New("lower0")
	lower1Fsys := memfs.New("lower1")

	top := canonicalSource(t, topFsys)
	lower0 := canonicalSource(t, lower0Fsys)
	lower1 := NewDataSource[vroot.File](lower1Fsys)

	seed(t, top, "dir/from-top")
	seed(t, lower0, "dir/from-lower0")
	seed(t, lower1, "dir/from-lower1", "masked-deeper", "gone")

	mark(t, "SetWhiteout", lower0.store.SetWhiteout("masked-deeper"))

	probes := []string{
		"",
		"dir",
		"dir/from-top",
		"dir/from-lower0",
		"dir/from-lower1",
		"masked-deeper",
		"gone",
	}

	s := newState(t, top, lower0, lower1)
	mutate(t, s, "setWhiteout", func() error { return s.setWhiteout("gone") })
	mutate(t, s, "setOpaque", func() error { return s.setOpaque("dir") })
	before := visibility(t, s, probes...)

	for _, d := range []*DataSource{top, lower0, lower1} {
		if err := d.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}

	again := newState(
		t,
		canonicalSource(t, topFsys),
		canonicalSource(t, lower0Fsys),
		NewDataSource[vroot.File](lower1Fsys),
	)
	if after := visibility(t, again, probes...); !slices.Equal(before, after) {
		t.Errorf("visibility after rebuilding = %q, want %q", after, before)
	}
	// The rebuilt state answers the same because the masks came back, not because
	// nothing was masked.
	assertEntries(t, again, "dir", "from-top")
	assertLookupErr(t, again, "gone", fs.ErrNotExist)
	assertLookupErr(t, again, "masked-deeper", fs.ErrNotExist)
}
