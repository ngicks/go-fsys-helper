// Package acceptancetest provides reusable contract tests for overlayfs
// pluggable components such as [overlayfs.MetadataStore].
package acceptancetest

import (
	"slices"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot/overlayfs"
)

// ReopenFunc returns a fresh [overlayfs.MetadataStore] over the same durable
// backing as the store it was returned alongside, for replay assertions. A
// nil ReopenFunc marks a non-durable store (durability sub-tests are skipped).
type ReopenFunc func() overlayfs.MetadataStore

// NewStoreFunc builds a fresh store and, for durable stores, a [ReopenFunc].
type NewStoreFunc func(t *testing.T) (store overlayfs.MetadataStore, reopen ReopenFunc)

// RunMetadataStore runs the journal contract against a store.
//
// newStore returns a fresh store and, for durable stores, a reopen function
// that returns a FRESH store over the SAME durable backing (used for
// durability/replay assertions). Pass a nil reopen (return nil) for
// non-durable stores such as [overlayfs.MetadataStoreMem]; the durability
// sub-tests are then skipped.
//
// If a store implements io.Closer, the harness does NOT close it; callers that
// need Close should arrange it via t.Cleanup in newStore.
func RunMetadataStore(t *testing.T, newStore NewStoreFunc) {
	t.Helper()

	t.Run("whiteout_set_clear_reflected_in_load", func(t *testing.T) {
		store, _ := newStore(t)
		mustNoErr(t, store.SetWhiteout("dir/removed.txt"))
		w, o := mustLoad(t, store)
		assertContains(t, "whiteouts", w, "dir/removed.txt")
		assertEmpty(t, "opaques", o)

		mustNoErr(t, store.ClearWhiteout("dir/removed.txt"))
		w, _ = mustLoad(t, store)
		assertNotContains(t, "whiteouts", w, "dir/removed.txt")
	})

	t.Run("opaque_set_clear_reflected_in_load", func(t *testing.T) {
		store, _ := newStore(t)
		mustNoErr(t, store.SetOpaque("recreated-dir"))
		w, o := mustLoad(t, store)
		assertContains(t, "opaques", o, "recreated-dir")
		assertEmpty(t, "whiteouts", w)

		mustNoErr(t, store.ClearOpaque("recreated-dir"))
		_, o = mustLoad(t, store)
		assertNotContains(t, "opaques", o, "recreated-dir")
	})

	t.Run("last_writer_wins_set_then_clear_absent", func(t *testing.T) {
		store, _ := newStore(t)
		mustNoErr(t, store.SetWhiteout("a/b"))
		mustNoErr(t, store.ClearWhiteout("a/b"))
		w, _ := mustLoad(t, store)
		assertNotContains(t, "whiteouts", w, "a/b")
	})

	t.Run("last_writer_wins_clear_then_set_present", func(t *testing.T) {
		store, _ := newStore(t)
		mustNoErr(t, store.ClearWhiteout("a/b"))
		mustNoErr(t, store.SetWhiteout("a/b"))
		w, _ := mustLoad(t, store)
		assertContains(t, "whiteouts", w, "a/b")
	})

	t.Run("whiteout_and_opaque_independent_namespaces", func(t *testing.T) {
		store, _ := newStore(t)
		mustNoErr(t, store.SetWhiteout("shared"))
		mustNoErr(t, store.SetOpaque("shared"))
		w, o := mustLoad(t, store)
		assertContains(t, "whiteouts", w, "shared")
		assertContains(t, "opaques", o, "shared")

		// Clearing the whiteout must not affect the opaque.
		mustNoErr(t, store.ClearWhiteout("shared"))
		w, o = mustLoad(t, store)
		assertNotContains(t, "whiteouts", w, "shared")
		assertContains(t, "opaques", o, "shared")
	})

	t.Run("path_validation_rejects_root_empty_dotdot", func(t *testing.T) {
		store, _ := newStore(t)
		for _, bad := range []string{".", "", "..", "a/../..", "../x", "a/../../b"} {
			if err := store.SetWhiteout(bad); err == nil {
				t.Errorf("SetWhiteout(%q): expected error, got nil", bad)
			}
			if err := store.SetOpaque(bad); err == nil {
				t.Errorf("SetOpaque(%q): expected error, got nil", bad)
			}
			if err := store.ClearWhiteout(bad); err == nil {
				t.Errorf("ClearWhiteout(%q): expected error, got nil", bad)
			}
			if err := store.ClearOpaque(bad); err == nil {
				t.Errorf("ClearOpaque(%q): expected error, got nil", bad)
			}
		}
	})

	t.Run("idempotent_set_and_clear", func(t *testing.T) {
		store, _ := newStore(t)
		mustNoErr(t, store.SetWhiteout("x"))
		mustNoErr(t, store.SetWhiteout("x"))
		w, _ := mustLoad(t, store)
		if got := countEq(w, "x"); got != 1 {
			t.Errorf("after double SetWhiteout, want exactly one 'x', got %d (%v)", got, w)
		}
		mustNoErr(t, store.ClearWhiteout("x"))
		mustNoErr(t, store.ClearWhiteout("x")) // clearing absent is a no-op
		w, _ = mustLoad(t, store)
		assertNotContains(t, "whiteouts", w, "x")
	})

	t.Run("path_normalization_round_trips_clean", func(t *testing.T) {
		store, _ := newStore(t)
		// "a/./b" cleans to "a/b".
		mustNoErr(t, store.SetWhiteout("a/./b"))
		w, _ := mustLoad(t, store)
		assertContains(t, "whiteouts", w, "a/b")
	})

	t.Run("durable_replay_after_flush_and_reopen", func(t *testing.T) {
		store, reopen := newStore(t)
		if reopen == nil {
			t.Skip("non-durable store; skipping replay assertions")
		}
		mustNoErr(t, store.SetWhiteout("dir/a.txt"))
		mustNoErr(t, store.SetWhiteout("dir/b.txt"))
		mustNoErr(t, store.SetOpaque("recreated"))
		mustNoErr(t, store.ClearWhiteout("dir/b.txt"))
		mustNoErr(t, store.Flush())

		fresh := reopen()
		w, o := mustLoad(t, fresh)
		assertContains(t, "whiteouts", w, "dir/a.txt")
		assertNotContains(t, "whiteouts", w, "dir/b.txt")
		assertContains(t, "opaques", o, "recreated")
	})

	t.Run("durable_replay_preserves_full_live_set", func(t *testing.T) {
		store, reopen := newStore(t)
		if reopen == nil {
			t.Skip("non-durable store; skipping replay assertions")
		}
		want := []string{"a", "b/c", "b/d", "e/f/g"}
		for _, p := range want {
			mustNoErr(t, store.SetWhiteout(p))
		}
		mustNoErr(t, store.Flush())
		fresh := reopen()
		w, _ := mustLoad(t, fresh)
		got := slices.Clone(w)
		slices.Sort(got)
		exp := slices.Clone(want)
		slices.Sort(exp)
		if !slices.Equal(got, exp) {
			t.Errorf("replayed whiteouts = %v, want %v", got, exp)
		}
	})
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func mustLoad(t *testing.T, s overlayfs.MetadataStore) (whiteouts, opaques []string) {
	t.Helper()
	w, o, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return w, o
}

func assertContains(t *testing.T, kind string, got []string, want string) {
	t.Helper()
	if !slices.Contains(got, want) {
		t.Errorf("%s: expected to contain %q, got %v", kind, want, got)
	}
}

func assertNotContains(t *testing.T, kind string, got []string, notWant string) {
	t.Helper()
	if slices.Contains(got, notWant) {
		t.Errorf("%s: expected NOT to contain %q, got %v", kind, notWant, got)
	}
}

func assertEmpty(t *testing.T, kind string, got []string) {
	t.Helper()
	if len(got) != 0 {
		t.Errorf("%s: expected empty, got %v", kind, got)
	}
}

func countEq(s []string, v string) int {
	n := 0
	for _, x := range s {
		if x == v {
			n++
		}
	}
	return n
}
