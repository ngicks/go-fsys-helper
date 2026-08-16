package overlayfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// What every mutating op shares; the ops themselves are in chmod.go, open.go,
// mkdir.go, remove.go, rename.go and link.go.
//
// Each of them takes the state write lock for its whole run and calls the
// lock-free helpers under it, so the lookup a change is decided from and the
// masking that change records land as one step (D16).
//
// Nothing is ever written to a layer to record masking: no ".wh." file, no
// character device, no opaque sentinel (D6). Whiteout and opaque go through
// [overlayState]'s mutators, which commit to the top's [MetadataStore] before
// they touch memory, so the state is durable by the time the call returns (D14).

// top is the layer every write lands in.
func (f *Fs) top() *DataSource { return f.state.layers[0] }

// topPath maps a merged-view name onto its path in the top's content tree.
func (f *Fs) topPath(name string) string { return f.state.layerPath(0, name) }

// copyUp is the gate every mutating op passes: whatever name resolves to is
// brought into the top, so the op acts on a copy of the overlay's own (§4.3).
//
// A name the top already holds is left alone. That re-check is what stands in
// for a singleflight — the write lock serializes the copies, and the second
// writer of a name finds the first one's copy already there (D16). A name no
// layer holds is left alone too: the caller is creating it, and there is nothing
// to copy.
//
// Masking is not the policy's business and barely this method's: a copy-up
// changes which layer a name is read from, not whether it is visible, so what
// masked it before masks it after. The one exception is an opaque mark left over
// on a name the top did not hold, which would start hiding the lower children of
// a directory that was merged a moment ago.
//
// The caller holds the write lock.
func (f *Fs) copyUp(name string) error {
	if name == "" {
		return nil
	}
	res, err := f.state.lookup(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case res.layer == 0:
		return nil
	}
	from, err := res.src.contentView()
	if err != nil {
		return err
	}
	if err := f.policy.CopyUp(from, f.top(), name); err != nil {
		return err
	}
	if _, opaque := f.state.marks(0, name); opaque {
		return f.state.clearOpaque(name)
	}
	return nil
}

// ensureParent establishes what a create needs before it writes: name's parent is
// a directory in the merged view, and the top holds it.
//
// The directories materialized on the way are ordinary ones. An opaque mark
// would hide the lower children of a directory this only passed through on its
// way to a file (D7).
func (f *Fs) ensureParent(name string) error {
	parent := parentName(name)
	res, err := f.state.lookup(parent)
	if err != nil {
		return err
	}
	if !res.info.IsDir() {
		return syscall.ENOTDIR
	}
	return f.copyUp(parent)
}

// unmask lifts the whiteout standing on name, which is what putting content back
// where a removal masked it has to do before the name means anything again.
func (f *Fs) unmask(name string) error {
	if whiteout, _ := f.state.marks(0, name); !whiteout {
		return nil
	}
	return f.state.clearWhiteout(name)
}

// mutateTop runs op against name's path in the top, having resolved name over the
// merged view and copied up whatever it lands on. skipLast leaves the final
// component unresolved, which is what an op addressing a symlink itself wants.
func (f *Fs) mutateTop(
	name string,
	skipLast bool,
	op func(fsys vroot.Fs[vroot.File], path string) error,
) error {
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	full, err := f.resolve(name, skipLast)
	if err != nil {
		return err
	}
	return f.mutateLocked(full, op)
}

// mutateResolved is [Fs.mutateTop] for a name already resolved over the merged
// view, which is what a handle the overlay handed out holds.
func (f *Fs) mutateResolved(
	full string,
	op func(fsys vroot.Fs[vroot.File], path string) error,
) error {
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	return f.mutateLocked(full, op)
}

// openTopDir opens the top's copy of an already-resolved directory name, which
// is what a handle onto that directory is re-pointed at once a copy-up has
// brought it into the top.
func (f *Fs) openTopDir(full string) (vroot.File, error) {
	f.state.mu.RLock()
	defer f.state.mu.RUnlock()
	return f.top().fsys.OpenFile(f.topPath(full), os.O_RDONLY, 0)
}

func (f *Fs) mutateLocked(
	full string,
	op func(fsys vroot.Fs[vroot.File], path string) error,
) error {
	if err := f.copyUp(full); err != nil {
		return err
	}
	return op(f.top().fsys, f.topPath(full))
}

// namesSelf reports whether name addresses the view's own root, which a
// structural op cannot take as a target: [os.RemoveAll] answers EINVAL for ".",
// and a sub-overlay from [Fs.OpenRoot] must no more be able to remove or rename
// the directory it is confined to than the overlay can its own.
func namesSelf(name string) bool {
	return filepath.Clean(name) == "."
}

// ancestry lists name and every directory above it, shallowest first, leaving out
// the root.
func ancestry(name string) []string {
	var out []string
	for n := name; n != ""; n = parentName(n) {
		out = append(out, n)
	}
	slices.Reverse(out)
	return out
}
