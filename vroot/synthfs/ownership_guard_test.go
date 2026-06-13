package synthfs_test

import (
	"os"
	"testing"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil/testhelper"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/synthfs"
)

// --- D10: single ownership of file mode (node owns it; Chmod through an open
// handle must be visible to the tree's Stat). ---

// TestHandleChmodVisibleToStat covers plan 01 V3 / decision D10: a Chmod issued
// through an open file handle must be visible to a subsequent tree Stat and to
// the handle's own Stat, because the node owns the mode (the view is
// authoritative only for Size). Pre-V3, memHandle.Chmod wrote a buffer-local
// mode that the tree's Stat never composed.
func TestHandleChmodVisibleToStat(t *testing.T) {
	r := synthfs.NewRoot("synth://", nil)
	c := testhelper.New(t, r)
	c.SetupLines(`f.txt: "hello"`)

	f, err := r.OpenFile("f.txt", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()

	if err := f.Chmod(0o600); err != nil {
		t.Fatalf("handle Chmod: %v", err)
	}

	// Tree Stat must reflect the handle Chmod.
	info, err := r.Stat("f.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("tree Stat mode after handle Chmod: got %o, want 0600", got)
	}

	// Handle Stat must agree.
	hinfo, err := f.Stat()
	if err != nil {
		t.Fatalf("handle Stat: %v", err)
	}
	if got := hinfo.Mode().Perm(); got != 0o600 {
		t.Errorf("handle Stat mode after handle Chmod: got %o, want 0600", got)
	}

	// Size must still be read live from the view (handle Stat composes node mode
	// with view size).
	if hinfo.Size() != int64(len("hello")) {
		t.Errorf("handle Stat size: got %d, want %d", hinfo.Size(), len("hello"))
	}
}

// TestHandleChmodThenTreeChmod confirms a tree-level Chmod and a handle-level
// Chmod target the same field (no divergence): the last writer wins regardless
// of which surface issued it.
func TestHandleChmodThenTreeChmod(t *testing.T) {
	r := synthfs.NewRoot("synth://", nil)
	c := testhelper.New(t, r)
	c.SetupLines(`f.txt: "x"`)

	f, err := r.OpenFile("f.txt", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()

	if err := f.Chmod(0o700); err != nil {
		t.Fatalf("handle Chmod: %v", err)
	}
	if err := r.Chmod("f.txt", 0o640); err != nil {
		t.Fatalf("tree Chmod: %v", err)
	}
	hinfo, _ := f.Stat()
	if got := hinfo.Mode().Perm(); got != 0o640 {
		t.Errorf("handle Stat after tree Chmod: got %o, want 0640", got)
	}
}

// --- D9: uniform DisableOpenFileRemoval guard across every unlink path. ---

func guardedRoot(t *testing.T, lines ...string) *synthfs.Root {
	t.Helper()
	r := synthfs.NewRoot("synth://", &synthfs.Option{DisableOpenFileRemoval: true})
	testhelper.New(t, r).SetupLines(lines...)
	return r
}

func openHandle(t *testing.T, r *synthfs.Root, name string) vroot.File {
	t.Helper()
	f, err := r.Open(name)
	if err != nil {
		t.Fatalf("Open %q: %v", name, err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// assertGuardFired checks the guard rejected an unlink (the error is non-nil
// whenever the sharing-violation guard engages; the concrete sentinel differs by
// GOOS so only non-nil is asserted portably).
func assertGuardFired(t *testing.T, err error, op string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: want sharing-violation error with an open handle, got nil", op)
	}
}

// TestGuardRemoveOpenFile is the baseline: Remove already honored the guard.
func TestGuardRemoveOpenFile(t *testing.T) {
	r := guardedRoot(t, `f.txt: "x"`)
	openHandle(t, r, "f.txt")
	assertGuardFired(t, r.Remove("f.txt"), "Remove")
	if _, err := r.Stat("f.txt"); err != nil {
		t.Errorf("file should survive a blocked Remove: %v", err)
	}
}

// TestGuardRemoveAllOpenFile is NEW behavior (plan 01 V3 / D9): RemoveAll now
// honors the guard, and rejects the whole subtree before deleting any of it.
func TestGuardRemoveAllOpenFile(t *testing.T) {
	r := guardedRoot(t,
		"d/",
		"d/sub/",
		`d/sub/open.txt: "x"`,
		`d/other.txt: "y"`,
	)
	openHandle(t, r, "d/sub/open.txt")
	assertGuardFired(t, r.RemoveAll("d"), "RemoveAll")
	// Nothing in the subtree may have been deleted (atomic rejection).
	for _, p := range []string{"d", "d/sub", "d/sub/open.txt", "d/other.txt"} {
		if _, err := r.Stat(p); err != nil {
			t.Errorf("%q should survive a blocked RemoveAll: %v", p, err)
		}
	}
}

// TestGuardRenameOverOpenFile is NEW behavior (plan 01 V3 / D9): renaming over a
// still-open destination file now honors the guard.
func TestGuardRenameOverOpenFile(t *testing.T) {
	r := guardedRoot(t,
		`src.txt: "fresh"`,
		`dst.txt: "stale"`,
	)
	openHandle(t, r, "dst.txt") // hold the destination open
	assertGuardFired(t, r.Rename("src.txt", "dst.txt"), "Rename-over")
	if _, err := r.Stat("src.txt"); err != nil {
		t.Errorf("src should survive a blocked Rename-over: %v", err)
	}
	if _, err := r.Stat("dst.txt"); err != nil {
		t.Errorf("dst should survive a blocked Rename-over: %v", err)
	}
}

// TestGuardAddFileOverrideOpenFile covers the ingest override path (plan 01 V3 /
// D9): AddFile override of a still-open file honors the guard.
func TestGuardAddFileOverrideOpenFile(t *testing.T) {
	r := guardedRoot(t, `f.txt: "old"`)
	openHandle(t, r, "f.txt")
	view := synthfs.NewBytesView([]byte("new"), 0o644, time.Now())
	assertGuardFired(t, r.AddFile("f.txt", view, synthfs.MergeOverwrite), "AddFile-override")
	// The existing file must survive.
	if _, err := r.Stat("f.txt"); err != nil {
		t.Errorf("file should survive a blocked AddFile override: %v", err)
	}
}

// TestGuardOffAllowsUnlink confirms the guard only engages when the option is
// set: with it off, the same operations succeed even with an open handle.
func TestGuardOffAllowsUnlink(t *testing.T) {
	r := synthfs.NewRoot("synth://", nil) // DisableOpenFileRemoval defaults off
	testhelper.New(t, r).SetupLines(`f.txt: "x"`, `dst.txt: "y"`, `src.txt: "z"`)

	openHandle(t, r, "f.txt")
	if err := r.Remove("f.txt"); err != nil {
		t.Errorf("Remove with guard off: %v", err)
	}

	openHandle(t, r, "dst.txt")
	if err := r.Rename("src.txt", "dst.txt"); err != nil {
		t.Errorf("Rename-over with guard off: %v", err)
	}
}
