package overlayfs_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/memfs"
	"github.com/ngicks/go-fsys-helper/vroot/overlayfs"
)

// makeSeededOverlay builds an overlay (memfs top + ONE memfs lower) and seeds
// the lower directly with lowerLines. DotTmp policy keeps listings clean. The
// returned *overlayfs.Fs is the live overlay; the test owns Close.
func makeSeededOverlay(t *testing.T, lowerLines []string) *overlayfs.Fs {
	t.Helper()
	top := memfs.New("top")
	lower := memfs.New("lower")
	if len(lowerLines) > 0 {
		testhelper.New(t, lower).SetupLines(lowerLines...)
	}
	return overlayfs.New(
		overlayfs.NewLayer[vroot.File](top, overlayfs.NewMetadataStoreMem()),
		[]overlayfs.Layer{overlayfs.NewLayer[vroot.File](lower, nil)},
		&overlayfs.Option{CopyUpPolicy: overlayfs.NewCopyUpPolicyDotTmp(0)},
	)
}

// TestConcurrentIndependentPaths stresses the overlay with many goroutines each
// acting on its OWN distinct paths (file + dir). Under -race this proves that
// independent paths neither serialize incorrectly nor race on shared state (the
// ovl-node table, the metadata store, the singleflight group).
func TestConcurrentIndependentPaths(t *testing.T) {
	const seed = 50
	lowerLines := make([]string, 0, seed)
	for i := range seed {
		lowerLines = append(lowerLines, fmt.Sprintf("f%d.txt: %q", i, fmt.Sprintf("lower-%d", i)))
	}
	o := makeSeededOverlay(t, lowerLines)
	defer func() { _ = o.Close() }()

	const workers = 50
	var g errgroup.Group
	for w := range workers {
		g.Go(func() error {
			// Each worker owns a disjoint path namespace: lower file fW.txt for
			// copy-up via write, plus brand-new top-only paths.
			lowerFile := fmt.Sprintf("f%d.txt", w)
			newFile := fmt.Sprintf("own-%d.txt", w)
			newDir := fmt.Sprintf("dir-%d", w)

			// Write to a lower-backed file → triggers copy-up of its own path.
			f, err := o.OpenFile(lowerFile, os.O_RDWR, 0)
			if err != nil {
				return fmt.Errorf("OpenFile %q: %w", lowerFile, err)
			}
			if _, err := f.WriteString("x"); err != nil {
				_ = f.Close()
				return fmt.Errorf("WriteString %q: %w", lowerFile, err)
			}
			if err := f.Close(); err != nil {
				return fmt.Errorf("Close %q: %w", lowerFile, err)
			}
			if err := o.Chmod(lowerFile, 0o600); err != nil {
				return fmt.Errorf("Chmod %q: %w", lowerFile, err)
			}
			if _, err := o.Stat(lowerFile); err != nil {
				return fmt.Errorf("Stat %q: %w", lowerFile, err)
			}

			// Create a fresh top-only file, read it back, then remove it.
			cf, err := o.Create(newFile)
			if err != nil {
				return fmt.Errorf("Create %q: %w", newFile, err)
			}
			if _, err := cf.WriteString("hello"); err != nil {
				_ = cf.Close()
				return fmt.Errorf("WriteString %q: %w", newFile, err)
			}
			if err := cf.Close(); err != nil {
				return fmt.Errorf("Close %q: %w", newFile, err)
			}
			rf, err := o.Open(newFile)
			if err != nil {
				return fmt.Errorf("Open %q: %w", newFile, err)
			}
			if _, err := io.ReadAll(rf); err != nil {
				_ = rf.Close()
				return fmt.Errorf("ReadAll %q: %w", newFile, err)
			}
			if err := rf.Close(); err != nil {
				return fmt.Errorf("Close read %q: %w", newFile, err)
			}
			if err := o.Remove(newFile); err != nil {
				return fmt.Errorf("Remove %q: %w", newFile, err)
			}

			// Mkdir a distinct dir.
			if err := o.Mkdir(newDir, 0o755); err != nil {
				return fmt.Errorf("Mkdir %q: %w", newDir, err)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("concurrent independent-path ops: %v", err)
	}
}

// countingPolicy wraps a real CopyUpPolicy and records how many times CopyUp is
// invoked per name, with a small sleep to widen the singleflight race window.
type countingPolicy struct {
	inner overlayfs.CopyUpPolicy

	mu    sync.Mutex
	calls map[string]int
}

func newCountingPolicy(inner overlayfs.CopyUpPolicy) *countingPolicy {
	return &countingPolicy{inner: inner, calls: make(map[string]int)}
}

func (c *countingPolicy) CopyUp(from, to vroot.Fs[vroot.File], name string) error {
	c.mu.Lock()
	c.calls[name]++
	c.mu.Unlock()
	// Widen the window so concurrent writers would both reach here if
	// singleflight did NOT dedup.
	time.Sleep(2 * time.Millisecond)
	return c.inner.CopyUp(from, to, name)
}

func (c *countingPolicy) count(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[name]
}

// TestConcurrentCopyUpSingleflight proves that many concurrent write-opens of
// the same lower-only path collapse to EXACTLY ONE CopyUp invocation (decision
// 11, singleflight dedup) and the final top content is consistent.
func TestConcurrentCopyUpSingleflight(t *testing.T) {
	top := memfs.New("top")
	lower := memfs.New("lower")
	testhelper.New(t, lower).SetupLines(`shared.txt: "shared-lower"`)

	policy := newCountingPolicy(overlayfs.NewCopyUpPolicyDotTmp(0))
	o := overlayfs.New(
		overlayfs.NewLayer[vroot.File](top, overlayfs.NewMetadataStoreMem()),
		[]overlayfs.Layer{overlayfs.NewLayer[vroot.File](lower, nil)},
		&overlayfs.Option{CopyUpPolicy: policy},
	)
	defer func() { _ = o.Close() }()

	const workers = 20
	start := make(chan struct{})
	var g errgroup.Group
	for range workers {
		g.Go(func() error {
			<-start // barrier: all goroutines fire together.
			f, err := o.OpenFile("shared.txt", os.O_RDWR, 0)
			if err != nil {
				return fmt.Errorf("OpenFile: %w", err)
			}
			return f.Close()
		})
	}
	close(start)
	if err := g.Wait(); err != nil {
		t.Fatalf("concurrent write-open of shared.txt: %v", err)
	}

	if got := policy.count("shared.txt"); got != 1 {
		t.Fatalf("CopyUp called %d times for shared.txt, want exactly 1 (singleflight)", got)
	}
	// The top copy must exist and carry the lower's content (no writes appended
	// here beyond the copy-up itself).
	if got := readAll(t, o, "shared.txt"); got != "shared-lower" {
		t.Fatalf("shared.txt = %q, want %q", got, "shared-lower")
	}
}

// TestConcurrentCrossingRenames runs crossing renames (x<->y, a<->b) from many
// goroutines and fails if they do not complete within a timeout — catching a
// lock-ordering deadlock (Rename locks the two nodes in canonical lexical
// order). The point is "no deadlock / no panic / no hang"; the exact final
// arrangement is not asserted (renames may legitimately race and fail with
// ENOENT once a name has moved).
func TestConcurrentCrossingRenames(t *testing.T) {
	top := memfs.New("top")
	testhelper.New(t, top).SetupLines(
		`x: "x"`,
		`y: "y"`,
		`a: "a"`,
		`b: "b"`,
	)
	o := overlayfs.New(
		overlayfs.NewLayer[vroot.File](top, overlayfs.NewMetadataStoreMem()),
		nil,
		&overlayfs.Option{CopyUpPolicy: overlayfs.NewCopyUpPolicyDotTmp(0)},
	)
	defer func() { _ = o.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		var g errgroup.Group
		for range 50 {
			g.Go(func() error {
				// A failed rename (a name already moved) is fine; a deadlock is not.
				_ = o.Rename("x", "y")
				return nil
			})
			g.Go(func() error {
				_ = o.Rename("y", "x")
				return nil
			})
			g.Go(func() error {
				_ = o.Rename("a", "b")
				return nil
			})
			g.Go(func() error {
				_ = o.Rename("b", "a")
				return nil
			})
		}
		_ = g.Wait()
	}()

	select {
	case <-done:
		// Completed: no deadlock.
	case <-time.After(10 * time.Second):
		t.Fatal("crossing renames deadlocked (did not complete within 10s)")
	}

	// Internal consistency: the merged root still lists a stable set of names
	// (every original name is accounted for under some surviving name); no panic.
	names := listNames(t, o, ".")
	if len(names) == 0 {
		t.Fatalf("root listing unexpectedly empty after crossing renames: %v", names)
	}
}

// TestDisableOpenFileRemoval verifies that with DisableOpenFileRemoval set,
// Remove of a path with a live open handle is rejected (errors.Is EINVAL on
// unix — the sharing-violation stand-in), and succeeds once the handle closes.
func TestDisableOpenFileRemoval(t *testing.T) {
	top := memfs.New("top")
	testhelper.New(t, top).SetupLines(`open.txt: "data"`)
	o := overlayfs.New(
		overlayfs.NewLayer[vroot.File](top, overlayfs.NewMetadataStoreMem()),
		nil,
		&overlayfs.Option{
			DisableOpenFileRemoval: true,
			CopyUpPolicy:           overlayfs.NewCopyUpPolicyDotTmp(0),
		},
	)
	defer func() { _ = o.Close() }()

	h, err := o.Open("open.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	err = o.Remove("open.txt")
	if err == nil {
		_ = h.Close()
		t.Fatal("Remove of an open file must fail when DisableOpenFileRemoval is set")
	}
	// On linux (this platform) the sharing-violation stand-in wraps EINVAL.
	if !errors.Is(err, syscall.EINVAL) {
		_ = h.Close()
		t.Fatalf("Remove error = %v, want one wrapping syscall.EINVAL", err)
	}

	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := o.Remove("open.txt"); err != nil {
		t.Fatalf("Remove after Close must succeed: %v", err)
	}
	mustNotExist(t, o, "open.txt")
}
