// Package acceptancetest defines the contract tests an
// [overlayfs.MetadataStore] implementation must pass.
package acceptancetest

import (
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot-adapter/overlayfs"
)

// OpenStore opens a store over one backing. Every call must address the same
// backing so that a store closed and opened again reads what the previous one
// wrote; a store is only opened again after [overlayfs.MetadataStore.Close]
// has returned, so an implementation may hold its backing exclusively.
//
// It takes a [testing.TB] rather than a [testing.T] so the same opener can
// serve benchmarks.
type OpenStore func(tb testing.TB) overlayfs.MetadataStore

// RunMetadataStore runs the [overlayfs.MetadataStore] contract.
//
// newBacking must hand back an opener over a fresh, empty backing on every
// call: subtests share no state, and several of them close the store and open
// it again to check that the state outlived the connection.
func RunMetadataStore(t *testing.T, newBacking func(tb testing.TB) OpenStore) {
	t.Run("Whiteout round trip", func(t *testing.T) { testWhiteoutRoundTrip(t, newBacking(t)) })
	t.Run("Opaque round trip", func(t *testing.T) { testOpaqueRoundTrip(t, newBacking(t)) })
	t.Run("Both marks", func(t *testing.T) { testBothMarks(t, newBacking(t)) })
	t.Run("Clear unset", func(t *testing.T) { testClearUnset(t, newBacking(t)) })
	t.Run("Normalization", func(t *testing.T) { testNormalization(t, newBacking(t)) })
	t.Run(
		"Whiteout drops subtree",
		func(t *testing.T) { testWhiteoutDropsSubtree(t, newBacking(t)) },
	)
	t.Run(
		"Whiteout keeps siblings",
		func(t *testing.T) { testWhiteoutKeepsSiblings(t, newBacking(t)) },
	)
	t.Run("Opaque keeps subtree", func(t *testing.T) { testOpaqueKeepsSubtree(t, newBacking(t)) })
	t.Run("Root", func(t *testing.T) { testRoot(t, newBacking(t)) })
	t.Run("Durability", func(t *testing.T) { testDurability(t, newBacking(t)) })
}

// assertLoad checks Load against the wanted marks, order-insensitively.
func assertLoad(t *testing.T, store overlayfs.MetadataStore, wantWhiteouts, wantOpaques []string) {
	t.Helper()
	whiteouts, opaques, err := store.Load()
	testhelper.NilErr(t, err)
	slices.Sort(whiteouts)
	slices.Sort(opaques)
	if !slices.Equal(whiteouts, wantWhiteouts) {
		t.Errorf("whiteouts = %q, want %q", whiteouts, wantWhiteouts)
	}
	if !slices.Equal(opaques, wantOpaques) {
		t.Errorf("opaques = %q, want %q", opaques, wantOpaques)
	}
}

func testWhiteoutRoundTrip(t *testing.T, open OpenStore) {
	store := open(t)
	defer store.Close()

	assertLoad(t, store, nil, nil)

	testhelper.NilErr(t, store.SetWhiteout("a/b"))
	assertLoad(t, store, []string{"a/b"}, nil)

	testhelper.NilErr(t, store.SetWhiteout("a/b"))
	assertLoad(t, store, []string{"a/b"}, nil)

	testhelper.NilErr(t, store.SetWhiteout("c"))
	assertLoad(t, store, []string{"a/b", "c"}, nil)

	testhelper.NilErr(t, store.ClearWhiteout("a/b"))
	assertLoad(t, store, []string{"c"}, nil)

	testhelper.NilErr(t, store.ClearWhiteout("c"))
	assertLoad(t, store, nil, nil)
}

func testOpaqueRoundTrip(t *testing.T, open OpenStore) {
	store := open(t)
	defer store.Close()

	testhelper.NilErr(t, store.SetOpaque("a/b"))
	testhelper.NilErr(t, store.SetOpaque("a/b"))
	testhelper.NilErr(t, store.SetOpaque("c"))
	assertLoad(t, store, nil, []string{"a/b", "c"})

	testhelper.NilErr(t, store.ClearOpaque("a/b"))
	assertLoad(t, store, nil, []string{"c"})

	testhelper.NilErr(t, store.ClearOpaque("c"))
	assertLoad(t, store, nil, nil)
}

func testBothMarks(t *testing.T, open OpenStore) {
	store := open(t)
	defer store.Close()

	testhelper.NilErr(t, store.SetWhiteout("d"))
	testhelper.NilErr(t, store.SetOpaque("d"))
	assertLoad(t, store, []string{"d"}, []string{"d"})

	// Clearing one mark leaves the other, and the node with it.
	testhelper.NilErr(t, store.ClearWhiteout("d"))
	assertLoad(t, store, nil, []string{"d"})

	// A whiteout over an opaque directory drops the opaque mark.
	testhelper.NilErr(t, store.SetWhiteout("d"))
	assertLoad(t, store, []string{"d"}, nil)
}

func testClearUnset(t *testing.T, open OpenStore) {
	store := open(t)
	defer store.Close()

	testhelper.NilErr(t, store.ClearWhiteout("never/set"))
	testhelper.NilErr(t, store.ClearOpaque("never/set"))
	assertLoad(t, store, nil, nil)

	testhelper.NilErr(t, store.SetWhiteout("a"))
	testhelper.NilErr(t, store.ClearOpaque("a"))
	assertLoad(t, store, []string{"a"}, nil)
}

func testNormalization(t *testing.T, open OpenStore) {
	store := open(t)
	defer store.Close()

	for _, name := range []string{"a/b", "./a/b", "a/b/", "/a/b", "a/./b", "a/c/../b", "../a/b"} {
		testhelper.NilErr(t, store.SetWhiteout(name))
		assertLoad(t, store, []string{"a/b"}, nil)
		testhelper.NilErr(t, store.SetOpaque(name))
		assertLoad(t, store, []string{"a/b"}, []string{"a/b"})

		testhelper.NilErr(t, store.ClearWhiteout(name))
		assertLoad(t, store, nil, []string{"a/b"})
		testhelper.NilErr(t, store.ClearOpaque(name))
		assertLoad(t, store, nil, nil)
	}
}

func testWhiteoutDropsSubtree(t *testing.T, open OpenStore) {
	store := open(t)
	defer store.Close()

	testhelper.NilErr(t, store.SetWhiteout("dir/x"))
	testhelper.NilErr(t, store.SetWhiteout("dir/sub/deep/y"))
	testhelper.NilErr(t, store.SetOpaque("dir/sub"))
	testhelper.NilErr(t, store.SetWhiteout("keep"))
	testhelper.NilErr(t, store.SetOpaque("dir"))

	testhelper.NilErr(t, store.SetWhiteout("dir"))
	assertLoad(t, store, []string{"dir", "keep"}, nil)

	// Clearing the whiteout does not bring the dropped marks back.
	testhelper.NilErr(t, store.ClearWhiteout("dir"))
	assertLoad(t, store, []string{"keep"}, nil)
}

func testWhiteoutKeepsSiblings(t *testing.T, open OpenStore) {
	store := open(t)
	defer store.Close()

	// "." sorts below "/" and "0" right above it: the names bracketing the
	// subtree range have to survive.
	siblings := []string{"dir.", "dir0", "dir2", "directory"}
	for _, name := range siblings {
		testhelper.NilErr(t, store.SetWhiteout(name))
	}
	testhelper.NilErr(t, store.SetWhiteout("dir/x"))

	testhelper.NilErr(t, store.SetWhiteout("dir"))
	assertLoad(t, store, append([]string{"dir"}, siblings...), nil)
}

func testOpaqueKeepsSubtree(t *testing.T, open OpenStore) {
	store := open(t)
	defer store.Close()

	testhelper.NilErr(t, store.SetWhiteout("dir/x"))
	testhelper.NilErr(t, store.SetOpaque("dir/sub"))

	testhelper.NilErr(t, store.SetOpaque("dir"))
	assertLoad(t, store, []string{"dir/x"}, []string{"dir", "dir/sub"})
}

func testRoot(t *testing.T, open OpenStore) {
	store := open(t)

	for _, name := range []string{"", ".", "/", "./"} {
		testhelper.NilErr(t, store.SetOpaque(name))
		assertLoad(t, store, nil, []string{""})
		testhelper.NilErr(t, store.ClearOpaque(name))
		assertLoad(t, store, nil, nil)
	}

	testhelper.NilErr(t, store.SetOpaque(""))
	testhelper.NilErr(t, store.SetWhiteout("a"))
	testhelper.NilErr(t, store.SetOpaque("b/c"))

	// The root's subtree is every other node.
	testhelper.NilErr(t, store.SetWhiteout(""))
	assertLoad(t, store, []string{""}, nil)

	testhelper.NilErr(t, store.Close())
	reopened := open(t)
	defer reopened.Close()
	assertLoad(t, reopened, []string{""}, nil)
}

func testDurability(t *testing.T, open OpenStore) {
	store := open(t)

	testhelper.NilErr(t, store.SetWhiteout("a/b"))
	testhelper.NilErr(t, store.SetWhiteout("gone"))
	testhelper.NilErr(t, store.SetOpaque("a"))
	testhelper.NilErr(t, store.SetOpaque("a/b"))
	testhelper.NilErr(t, store.Close())

	store = open(t)
	assertLoad(t, store, []string{"a/b", "gone"}, []string{"a", "a/b"})

	testhelper.NilErr(t, store.ClearWhiteout("gone"))
	testhelper.NilErr(t, store.ClearOpaque("a/b"))
	testhelper.NilErr(t, store.Close())

	store = open(t)
	defer store.Close()
	assertLoad(t, store, []string{"a/b"}, []string{"a"})
}
