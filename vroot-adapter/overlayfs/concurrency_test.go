package overlayfs

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/ngicks/go-fsys-helper/vroot"
	"golang.org/x/sync/errgroup"
)

// newOverlayOption is [newOverlay] with an option in place of the defaults, over
// a canonical top and one canonical lower.
func newOverlayOption(t *testing.T, mk fsysMaker, opt *Option) (f *Fs, top, lower0 *DataSource) {
	t.Helper()
	top = canonicalSource(t, mk(t, "top"))
	lower0 = canonicalSource(t, mk(t, "lower0"))
	f, err := New(top, []*DataSource{lower0}, opt)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return f, top, lower0
}

// countingPolicy counts the copy-ups the overlay runs, which is what turns "the
// second writer finds the first one's copy" from something the code reads like
// into something a test can check.
type countingPolicy struct {
	inner CopyUpPolicy
	calls atomic.Int64
}

func (p *countingPolicy) CopyUp(from vroot.Fs[vroot.File], top *DataSource, name string) error {
	p.calls.Add(1)
	return p.inner.CopyUp(from, top, name)
}

// TestConcurrentIndependentPaths runs the whole write path at once over paths no
// two workers share: each makes a directory, copies up a lower-backed file of
// its own, renames inside its directory and takes the directory back down again.
//
// The paths are disjoint, so the end state is exactly what each worker left,
// while the state lock, the parent materialization and the store writes are
// shared by all of them.
func TestConcurrentIndependentPaths(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, lower0, _ := newOverlay(t, mk)

		const workers = 8
		for i := range workers {
			put(t, lower0, "lower/f"+strconv.Itoa(i), "lower", 0o644)
		}

		var g errgroup.Group
		for i := range workers {
			g.Go(func() error {
				id := strconv.Itoa(i)
				dir := "w" + id
				if err := f.Mkdir(dir, 0o777); err != nil {
					return err
				}
				lower := filepath.FromSlash("lower/f" + id)
				if _, err := vroot.ReadFile(f, lower); err != nil {
					return err
				}
				file, err := f.Create(lower)
				if err != nil {
					return err
				}
				if _, err := file.Write([]byte("top-" + id)); err != nil {
					_ = file.Close()
					return err
				}
				if err := file.Close(); err != nil {
					return err
				}
				// Closed before the rename below: os.OpenFile on Windows grants no
				// FILE_SHARE_DELETE, so a handle still out on the source denies the
				// DELETE access renaming it needs and the move comes back a sharing
				// violation.
				file, err = f.Create(filepath.Join(dir, "a"))
				if err != nil {
					return err
				}
				if err := file.Close(); err != nil {
					return err
				}
				if err := f.Rename(filepath.Join(dir, "a"), filepath.Join(dir, "b")); err != nil {
					return err
				}
				if err := f.Remove(filepath.Join(dir, "b")); err != nil {
					return err
				}
				return f.RemoveAll(dir)
			})
		}
		mustDo(t, "concurrent independent paths", g.Wait())

		for i := range workers {
			id := strconv.Itoa(i)
			name := "lower/f" + id
			assertRead(t, f, filepath.FromSlash(name), "top-"+id)
			assertBackedBy(t, f, name, 0)
			if got, ok := layerContent(t, lower0, name); !ok || got != "lower" {
				t.Errorf("lower copy of %q = %q (present=%t), want %q", name, got, ok, "lower")
			}
			assertNotExist(t, f, "w"+id)
		}
		// Every worker took its own directory back down, and a copy-up records no
		// masking, so nothing is left to mark.
		assertMarks(t, f, nil, nil)
		assertWorkEmpty(t, top)
	})
}

// TestConcurrentCopyUpSamePath sends every worker at one lower-backed file at
// once: the file is copied up exactly once however many writers arrive, the copy
// carries one writer's payload whole, and the layer it came from is left alone.
func TestConcurrentCopyUpSamePath(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		policy := &countingPolicy{inner: NewCopyUpPolicyWork()}
		f, top, lower0 := newOverlayOption(t, mk, &Option{CopyUpPolicy: policy})
		put(t, lower0, "shared.txt", "lower", 0o644)

		const writers = 8
		payloads := make([]string, writers)
		var g errgroup.Group
		for i := range writers {
			// Equal lengths: two writers land at offset 0 of the same file, so a
			// short payload could otherwise leave a long one's tail behind and read
			// as corruption that is only the test's own doing.
			payload := "payload-" + strconv.Itoa(i)
			payloads[i] = payload
			g.Go(func() error {
				file, err := f.Create("shared.txt")
				if err != nil {
					return err
				}
				_, err = file.Write([]byte(payload))
				return errors.Join(err, file.Close())
			})
		}
		mustDo(t, "concurrent writes to one lower-backed name", g.Wait())

		if got := policy.calls.Load(); got != 1 {
			t.Errorf("copy-ups of one lower-backed name = %d, want 1", got)
		}
		got := read(t, f, "shared.txt")
		if !slices.Contains(payloads, got) {
			t.Errorf("shared.txt = %q, want one of %q", got, payloads)
		}
		if inTop, ok := layerContent(t, top, "shared.txt"); !ok || inTop != got {
			t.Errorf("top copy = %q (present=%t), want %q", inTop, ok, got)
		}
		if inLower, ok := layerContent(t, lower0, "shared.txt"); !ok || inLower != "lower" {
			t.Errorf("lower copy = %q (present=%t), want %q", inLower, ok, "lower")
		}
		assertMarks(t, f, nil, nil)
		assertWorkEmpty(t, top)
	})
}

// TestConcurrentCrossingRenames crosses two renames over the same pair of names,
// each the other's destination.
//
// A rename destroys whatever stands at its destination, so which of the two
// names survives is the schedule's business; what is checked is that the run
// terminates and that the view it leaves agrees with itself — the listing and
// the lookups name the same files, and every file that is still there holds a
// payload it was seeded with rather than a mix of both.
func TestConcurrentCrossingRenames(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, top, lower0, _ := newOverlay(t, mk)
		put(t, lower0, "a", "A", 0o644)
		put(t, lower0, "b", "B", 0o644)

		const rounds = 200
		var g errgroup.Group
		for _, pair := range [][2]string{{"a", "b"}, {"b", "a"}} {
			g.Go(func() error {
				for range rounds {
					switch err := f.Rename(pair[0], pair[1]); {
					case err == nil, errors.Is(err, fs.ErrNotExist):
					default:
						return err
					}
				}
				return nil
			})
		}
		mustDo(t, "crossing renames", g.Wait())

		listed := entries(t, f, ".")
		if len(listed) == 0 {
			t.Fatal(
				"both names are gone; neither rename removes its source and its destination at once",
			)
		}
		for _, name := range []string{"a", "b"} {
			_, err := f.Lstat(name)
			switch {
			case err == nil && !slices.Contains(listed, name):
				t.Errorf("%q resolves but the listing %q does not show it", name, listed)
			case errors.Is(err, fs.ErrNotExist) && slices.Contains(listed, name):
				t.Errorf("the listing shows %q but it does not resolve", name)
			case err != nil && !errors.Is(err, fs.ErrNotExist):
				t.Fatalf("lstat %q: %v", name, err)
			case err == nil:
				if got := read(t, f, name); got != "A" && got != "B" {
					t.Errorf("%q = %q, want one of the seeded payloads", name, got)
				}
			}
		}
		assertWorkEmpty(t, top)
	})
}

// TestConcurrentOpenFileRemoval runs the removal gate against the opens it
// counts: each name has one worker opening and closing it and one worker trying
// to remove it, so a removal lands either between two opens or against a handle
// that is out.
//
// A refused removal is the only error the removers retry — anything else is a
// failure — and every name ends up removed, whited out and free of handles.
func TestConcurrentOpenFileRemoval(t *testing.T) {
	eachBacking(t, func(t *testing.T, mk fsysMaker) {
		f, _, lower0 := newOverlayOption(t, mk, &Option{DisableOpenFileRemoval: true})

		const (
			workers = 8
			opens   = 64
			// Bounded so a gate that never lets go fails instead of spinning. The
			// removers only ever wait for a handle that is already being closed.
			attempts = 10000
		)
		names := make([]string, workers)
		for i := range workers {
			names[i] = "f" + strconv.Itoa(i)
			put(t, lower0, names[i], "lower", 0o644)
		}

		var g errgroup.Group
		for _, name := range names {
			g.Go(func() error {
				for range opens {
					file, err := f.Open(name)
					if errors.Is(err, fs.ErrNotExist) {
						return nil
					}
					if err != nil {
						return err
					}
					if err := file.Close(); err != nil {
						return err
					}
				}
				return nil
			})
			g.Go(func() error {
				for range attempts {
					switch err := f.Remove(name); {
					case err == nil:
						return nil
					case errors.Is(err, errSharingViolation):
					default:
						return err
					}
				}
				return errors.New("removal of " + name + " never got through the gate")
			})
		}
		mustDo(t, "concurrent opens and removals", g.Wait())

		for _, name := range names {
			assertNotExist(t, f, name)
			if got := f.handles.count(name); got != 0 {
				t.Errorf("open handles on %q after every close = %d, want 0", name, got)
			}
		}
		assertMarks(t, f, names, nil)
	})
}
