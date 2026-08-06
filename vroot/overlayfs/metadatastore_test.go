package overlayfs_test

import (
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
	"github.com/ngicks/go-fsys-helper/vroot/overlayfs"
	"github.com/ngicks/go-fsys-helper/vroot/overlayfs/acceptancetest"
	"golang.org/x/sync/errgroup"
)

func TestMetadataStoreMem_Contract(t *testing.T) {
	acceptancetest.RunMetadataStore(
		t,
		func(t *testing.T) (overlayfs.MetadataStore, acceptancetest.ReopenFunc) {
			return overlayfs.NewMetadataStoreMem(), nil
		},
	)
}

// For the Log contract, back it with a single shared in-memory fs so reopen
// observes the same durable bytes.
func TestMetadataStoreLog_Contract_Batched(t *testing.T) {
	runLogContract(t, overlayfs.SyncBatched)
}

func TestMetadataStoreLog_Contract_PerOp(t *testing.T) {
	runLogContract(t, overlayfs.SyncPerOp)
}

func TestMetadataStoreLog_Contract_None(t *testing.T) {
	runLogContract(t, overlayfs.SyncNone)
}

func runLogContract(t *testing.T, mode overlayfs.SyncMode) {
	t.Helper()
	acceptancetest.RunMetadataStore(
		t,
		func(t *testing.T) (overlayfs.MetadataStore, acceptancetest.ReopenFunc) {
			fsys := memfs.New("meta")
			store := overlayfs.NewMetadataStoreLog(fsys, &overlayfs.LogOption{Sync: mode})
			t.Cleanup(func() { _ = store.Close() })
			// reopen shares the same backing fsys; the prior store must be flushed
			// (the contract test flushes before reopen).
			var reopened *overlayfs.MetadataStoreLog
			reopen := func() overlayfs.MetadataStore {
				reopened = overlayfs.NewMetadataStoreLog(fsys, &overlayfs.LogOption{Sync: mode})
				t.Cleanup(func() { _ = reopened.Close() })
				return reopened
			}
			return store, reopen
		},
	)
}

// readLog returns the raw bytes of the journal file.
func readLog(t *testing.T, fsys vroot.Fs[vroot.File]) []byte {
	t.Helper()
	f, err := fsys.Open(overlayfs.LogFileName)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	return b
}

func TestMetadataStoreLog_TrailingPartialLineTolerated(t *testing.T) {
	fsys := memfs.New("meta")
	store := overlayfs.NewMetadataStoreLog(fsys, &overlayfs.LogOption{Sync: overlayfs.SyncPerOp})
	if err := store.SetWhiteout("dir/a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetOpaque("recreated"); err != nil {
		t.Fatal(err)
	}
	if err := store.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// Manually append a partial (non-newline-terminated) line, simulating a
	// crash mid-append.
	f, err := fsys.OpenFile(overlayfs.LogFileName, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`+w "dir/partial`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen and Load: the partial line must be ignored, the complete records kept.
	reopened := overlayfs.NewMetadataStoreLog(fsys, &overlayfs.LogOption{Sync: overlayfs.SyncPerOp})
	defer func() { _ = reopened.Close() }()
	w, o, err := reopened.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !slices.Contains(w, "dir/a.txt") {
		t.Errorf("whiteouts %v missing dir/a.txt", w)
	}
	if slices.Contains(w, "dir/partial") {
		t.Errorf("whiteouts %v should not contain the partial line", w)
	}
	if !slices.Contains(o, "recreated") {
		t.Errorf("opaques %v missing recreated", o)
	}
}

func TestMetadataStoreLog_UnknownAndCommentLinesIgnored(t *testing.T) {
	fsys := memfs.New("meta")
	// Pre-seed a log file with header, comment, blank, unknown, and valid lines.
	f, err := fsys.OpenFile(overlayfs.LogFileName, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	content := "# vroot-overlayfs log v1\n" +
		"\n" +
		"# a comment\n" +
		`?x "weird"` + "\n" + // unknown record type
		`+w "kept.txt"` + "\n"
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	store := overlayfs.NewMetadataStoreLog(fsys, nil)
	defer func() { _ = store.Close() }()
	w, _, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(w, "kept.txt") {
		t.Errorf("whiteouts %v missing kept.txt", w)
	}
	if slices.Contains(w, "weird") {
		t.Errorf("whiteouts %v should not contain unknown-record path", w)
	}
}

func TestMetadataStoreLog_CompactionTriggersAndReplays(t *testing.T) {
	fsys := memfs.New("meta")
	// Low thresholds so compaction triggers quickly.
	store := overlayfs.NewMetadataStoreLog(fsys, &overlayfs.LogOption{
		Sync:          overlayfs.SyncPerOp,
		CompactFactor: 2,
		CompactMin:    4,
	})

	// Churn a single path many times: live set stays size 1, but appended grows
	// until > max(1*2, 4) = 4, forcing compaction.
	for i := range 50 {
		if i%2 == 0 {
			if err := store.SetWhiteout("churn"); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := store.ClearWhiteout("churn"); err != nil {
				t.Fatal(err)
			}
		}
	}
	// End on a Set so the live set has "churn".
	if err := store.SetWhiteout("churn"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetWhiteout("other"); err != nil {
		t.Fatal(err)
	}
	if err := store.Flush(); err != nil {
		t.Fatal(err)
	}

	// After compaction the file must be small: a header + the small live set,
	// not 50+ churn lines.
	raw := readLog(t, fsys)
	lines := nonEmptyLines(raw)
	if len(lines) > 8 {
		t.Errorf("expected compacted log to be small, got %d lines:\n%s", len(lines), raw)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := overlayfs.NewMetadataStoreLog(fsys, nil)
	defer func() { _ = reopened.Close() }()
	w, _, err := reopened.Load()
	if err != nil {
		t.Fatal(err)
	}
	got := slices.Clone(w)
	slices.Sort(got)
	if !slices.Equal(got, []string{"churn", "other"}) {
		t.Errorf("post-compaction live set = %v, want [churn other]", got)
	}
}

func TestMetadataStoreLog_CompactionDropsAncestorSubsumed(t *testing.T) {
	fsys := memfs.New("meta")
	store := overlayfs.NewMetadataStoreLog(fsys, &overlayfs.LogOption{
		Sync:          overlayfs.SyncPerOp,
		CompactFactor: 1,
		CompactMin:    1,
	})

	// Whiteout an ancestor dir and descendants under it; also an opaque under it.
	mustSet := func(set func(string) error, p string) {
		if err := set(p); err != nil {
			t.Fatalf("set %q: %v", p, err)
		}
	}
	mustSet(store.SetWhiteout, "deleted-dir")
	mustSet(store.SetWhiteout, "deleted-dir/child.txt")
	mustSet(store.SetWhiteout, "deleted-dir/sub/deep.txt")
	mustSet(store.SetOpaque, "deleted-dir/sub")
	mustSet(store.SetWhiteout, "kept.txt") // not under deleted-dir

	// Force a compaction by churning a single throwaway path: this inflates the
	// appended-line count beyond live*factor without growing the live set, so a
	// compaction fires AFTER all the real records above are present, exercising
	// ancestor-subsumption normalization on them.
	for range 20 {
		mustSet(store.SetWhiteout, "churn")
		mustSet(store.ClearWhiteout, "churn")
	}
	if err := store.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen: the live set still contains the ancestor + kept + spam, but the
	// descendant records subsumed by "deleted-dir" must have been dropped at
	// compaction.
	reopened := overlayfs.NewMetadataStoreLog(fsys, nil)
	defer func() { _ = reopened.Close() }()
	w, o, err := reopened.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(w, "deleted-dir") {
		t.Errorf("whiteouts %v missing ancestor deleted-dir", w)
	}
	if !slices.Contains(w, "kept.txt") {
		t.Errorf("whiteouts %v missing kept.txt", w)
	}
	if slices.Contains(w, "deleted-dir/child.txt") {
		t.Errorf("subsumed whiteout deleted-dir/child.txt should have been dropped: %v", w)
	}
	if slices.Contains(w, "deleted-dir/sub/deep.txt") {
		t.Errorf("subsumed whiteout deleted-dir/sub/deep.txt should have been dropped: %v", w)
	}
	if slices.Contains(o, "deleted-dir/sub") {
		t.Errorf("subsumed opaque deleted-dir/sub should have been dropped: %v", o)
	}
}

func TestMetadataStoreLog_BatchedDurabilityAfterFlush(t *testing.T) {
	fsys := memfs.New("meta")
	store := overlayfs.NewMetadataStoreLog(fsys, &overlayfs.LogOption{Sync: overlayfs.SyncBatched})

	if err := store.SetWhiteout("a"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetWhiteout("b"); err != nil {
		t.Fatal(err)
	}
	if err := store.Flush(); err != nil {
		t.Fatal(err)
	}

	// After Flush, a reopen of the same backing must see both.
	reopened := overlayfs.NewMetadataStoreLog(fsys, nil)
	defer func() { _ = reopened.Close() }()
	w, _, err := reopened.Load()
	if err != nil {
		t.Fatal(err)
	}
	got := slices.Clone(w)
	slices.Sort(got)
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("after Flush, reopened whiteouts = %v, want [a b]", got)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataStoreLog_CloseFlushesBatched(t *testing.T) {
	fsys := memfs.New("meta")
	store := overlayfs.NewMetadataStoreLog(fsys, &overlayfs.LogOption{Sync: overlayfs.SyncBatched})
	if err := store.SetWhiteout("flushed-on-close"); err != nil {
		t.Fatal(err)
	}
	// No explicit Flush — rely on Close to drain + persist.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := overlayfs.NewMetadataStoreLog(fsys, nil)
	defer func() { _ = reopened.Close() }()
	w, _, err := reopened.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(w, "flushed-on-close") {
		t.Errorf("Close did not flush the pending record; whiteouts = %v", w)
	}
}

func TestMetadataStoreLog_CloseIdempotent(t *testing.T) {
	fsys := memfs.New("meta")
	store := overlayfs.NewMetadataStoreLog(fsys, nil)
	if err := store.SetWhiteout("x"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestMetadataStore_ConcurrentAccess exercises genuine concurrent mutation +
// Load + Flush against both stores. Run under -race it must stay clean and the
// background writer (Batched) must not leak or race.
func TestMetadataStore_ConcurrentAccess(t *testing.T) {
	cases := []struct {
		name string
		make func() (overlayfs.MetadataStore, func())
	}{
		{
			name: "mem",
			make: func() (overlayfs.MetadataStore, func()) {
				return overlayfs.NewMetadataStoreMem(), func() {}
			},
		},
		{
			name: "log_batched",
			make: func() (overlayfs.MetadataStore, func()) {
				s := overlayfs.NewMetadataStoreLog(
					memfs.New("meta"),
					&overlayfs.LogOption{Sync: overlayfs.SyncBatched},
				)
				return s, func() { _ = s.Close() }
			},
		},
		{
			name: "log_perop",
			make: func() (overlayfs.MetadataStore, func()) {
				s := overlayfs.NewMetadataStoreLog(
					memfs.New("meta"),
					&overlayfs.LogOption{Sync: overlayfs.SyncPerOp},
				)
				return s, func() { _ = s.Close() }
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, cleanup := tc.make()
			defer cleanup()
			var g errgroup.Group
			for w := range 8 {
				g.Go(func() error {
					for i := range 64 {
						p := "g" + strconv.Itoa(w) + "/f" + strconv.Itoa(i)
						if err := store.SetWhiteout(p); err != nil {
							return err
						}
						if i%4 == 0 {
							if err := store.SetOpaque(p); err != nil {
								return err
							}
						}
						if i%3 == 0 {
							if _, _, err := store.Load(); err != nil {
								return err
							}
						}
						if i%8 == 0 {
							if err := store.Flush(); err != nil {
								return err
							}
						}
					}
					return nil
				})
			}
			if err := g.Wait(); err != nil {
				t.Fatalf("concurrent ops: %v", err)
			}
		})
	}
}

func TestSubMetadataStore(t *testing.T) {
	parent := overlayfs.NewMetadataStoreMem()
	sub := overlayfs.SubMetadataStore(parent, "subdir")

	if err := sub.SetWhiteout("removed.txt"); err != nil {
		t.Fatal(err)
	}
	if err := sub.SetOpaque("nested/dir"); err != nil {
		t.Fatal(err)
	}

	// Parent sees the prefixed keys.
	pw, po, err := parent.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(pw, "subdir/removed.txt") {
		t.Errorf("parent whiteouts %v missing subdir/removed.txt", pw)
	}
	if !slices.Contains(po, "subdir/nested/dir") {
		t.Errorf("parent opaques %v missing subdir/nested/dir", po)
	}

	// Sub sees the unprefixed keys.
	sw, so, err := sub.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(sw, "removed.txt") {
		t.Errorf("sub whiteouts %v missing removed.txt", sw)
	}
	if !slices.Contains(so, "nested/dir") {
		t.Errorf("sub opaques %v missing nested/dir", so)
	}

	// A sibling entry outside base must NOT appear in the sub view.
	if err := parent.SetWhiteout("other/x"); err != nil {
		t.Fatal(err)
	}
	if err := parent.SetWhiteout("subdirX/y"); err != nil { // prefix-collision guard
		t.Fatal(err)
	}
	sw, _, err = sub.Load()
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(sw, "other/x") || slices.Contains(sw, "x") {
		t.Errorf("sub view leaked sibling entry: %v", sw)
	}
	for _, p := range sw {
		if p == "subdirX/y" || p == "X/y" || p == "/y" {
			t.Errorf("sub view leaked prefix-collision entry %q: %v", p, sw)
		}
	}
}

func TestSubMetadataStore_RejectsEscape(t *testing.T) {
	sub := overlayfs.SubMetadataStore(overlayfs.NewMetadataStoreMem(), "base")
	for _, bad := range []string{"..", "../x", ".", ""} {
		if err := sub.SetWhiteout(bad); err == nil {
			t.Errorf("SubMetadataStore.SetWhiteout(%q): expected error", bad)
		}
	}
}

// nonEmptyLines returns the non-empty lines of raw.
func nonEmptyLines(raw []byte) []string {
	var out []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
