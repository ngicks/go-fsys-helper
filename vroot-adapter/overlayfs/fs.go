package overlayfs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

var _ vroot.Root[vroot.File, *Fs] = (*Fs)(nil)

// ErrTopReadOnly reports a canonical data source whose metadata store could only
// be opened for reading being handed to [New] as the top. Its layout is
// everything a top needs — the error is not [ErrNotCanonical] — but the first
// whiteout it has to record would fail, and by then the overlay has already
// served reads it cannot take back.
var ErrTopReadOnly = errors.New("top data source is read-only")

// Option configures an overlay. The zero value, and a nil *Option, select the
// defaults.
type Option struct {
	// CopyUpPolicy materializes a lower-backed name in the top before a write
	// reaches it. nil selects [NewCopyUpPolicyWork].
	CopyUpPolicy CopyUpPolicy
	// DisableOpenFileRemoval keeps a name the overlay still holds an open handle
	// on from being removed, the way Windows refuses to unlink an open file.
	// RemoveAll refuses when the handle is on the named path or anywhere under
	// it. Setting it on every platform is what makes an overlay behave the same
	// on all of them.
	DisableOpenFileRemoval bool
}

// Fs is the merged view of a canonical top over its lowers, as a [vroot.Root].
//
// A name resolves in mount order — top, then lowers[0], lowers[1], … — to the
// shallowest layer that shows it and is not masked for it. Reads are served from
// wherever the name resolved; a handle onto a lower is read-only, since a write
// through it would be the one route around the copy-up every write goes through.
//
// # Locking
//
// Each operation takes the state lock once for its whole run — read for a query,
// write for a change — and calls the lock-free methods of [overlayState] under
// it, so a lookup and whatever it decides on land as one step (D16). Data I/O on
// a handle the overlay already returned takes nothing: only the merged directory
// handle reaches back into the state, and it does so between calls rather than
// during one.
type Fs struct {
	state   *overlayState
	handles *handleTracker
	policy  CopyUpPolicy
	// disableOpenFileRemoval carries [Option.DisableOpenFileRemoval] for the
	// removal gate to consult.
	disableOpenFileRemoval bool
	// base is the sub-root an [Fs.OpenRoot] view addresses the shared state
	// through, in the spelling the masking state is keyed by; "" for the overlay
	// [New] built.
	base string
	// owner marks the overlay [New] built. It is the only one whose Close closes
	// the layers — a sub-overlay borrows what it was opened from.
	owner bool
}

// New stacks top over lowers and returns the merged view.
//
// top must be canonical: it holds the content the overlay writes, the staging
// its copy-ups pass through, and the store its masking is recorded in. A plain
// data source is refused with [ErrNotCanonical] and one whose store opened
// read-only with [ErrTopReadOnly].
//
// lowers are read-only and given in mount order: lowers[0] is the
// highest-priority lower, the way lowerdir=a:b:c reads, so a name resolves top →
// lowers[0] → lowers[1] → … (D20). A canonical lower goes on masking the layers
// stacked under it with the whiteouts its own store kept. A nil lower is refused
// with [fs.ErrInvalid], naming the index it was passed at.
//
// opt may be nil, which selects the defaults of [Option].
//
// New seeds the overlay's masking from every layer's store and empties the top's
// "work/" of whatever a previous run staged there and never published (D8).
// There is no Sync: masking is durable by the time the call that changed it
// returns (D14).
func New(top *DataSource, lowers []*DataSource, opt *Option) (*Fs, error) {
	if opt == nil {
		opt = &Option{}
	}
	state, err := newOverlayState(top, lowers)
	if err != nil {
		return nil, err
	}
	if top.readOnly {
		return nil, fmt.Errorf("overlayfs: %w: %s", ErrTopReadOnly, top.fsys.Name())
	}
	if err := top.sweepWork(); err != nil {
		return nil, err
	}
	policy := opt.CopyUpPolicy
	if policy == nil {
		policy = NewCopyUpPolicyWork()
	}
	return &Fs{
		state:                  state,
		handles:                newHandleTracker(),
		policy:                 policy,
		disableOpenFileRemoval: opt.DisableOpenFileRemoval,
		owner:                  true,
	}, nil
}

// IsRoot implements [vroot.Root]: no path leaves the overlay, and no path leaves
// a sub-overlay [Fs.OpenRoot] returned either.
func (f *Fs) IsRoot() {}

// Name reports the overlay together with the top layer's name, and the sub-root
// an [Fs.OpenRoot] view is confined to, in the form [vroot.PathPrefixFs] uses.
func (f *Fs) Name() string {
	name := "overlayfs: " + f.state.layers[0].fsys.Name()
	if f.base != "" {
		name = "base=" + f.base + ": " + name
	}
	return name
}

// Close closes every layer, and with it every layer's metadata store.
//
// Only the overlay [New] built owns the layers; Close on a sub-overlay from
// [Fs.OpenRoot] is a no-op. A sub-overlay shares the layers rather than holding
// any of its own, and closing them out from under the overlay it was opened from
// would break a view its caller never handed over.
func (f *Fs) Close() error {
	if !f.owner {
		return nil
	}
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	errs := make([]error, len(f.state.layers))
	for i, d := range f.state.layers {
		errs[i] = d.Close()
	}
	return errors.Join(errs...)
}

// Lstat reports name itself, following the symlinks on the way to it but not
// name when it is one.
func (f *Fs) Lstat(name string) (fs.FileInfo, error) {
	f.state.mu.RLock()
	defer f.state.mu.RUnlock()
	_, res, err := f.locate(name, true)
	if err != nil {
		return nil, fsutil.WrapPathErr("lstat", name, err)
	}
	return res.info, nil
}

// Stat reports what name resolves to, name itself included when it is a symlink.
func (f *Fs) Stat(name string) (fs.FileInfo, error) {
	f.state.mu.RLock()
	defer f.state.mu.RUnlock()
	_, res, err := f.locate(name, false)
	if err != nil {
		return nil, fsutil.WrapPathErr("stat", name, err)
	}
	return res.info, nil
}

// ReadLink returns the target name holds, read from the layer name resolves to.
func (f *Fs) ReadLink(name string) (string, error) {
	f.state.mu.RLock()
	defer f.state.mu.RUnlock()
	_, res, err := f.locate(name, true)
	if err != nil {
		return "", fsutil.WrapPathErr("readlink", name, err)
	}
	// Not checked against the mode first: the layer answers a name that is no
	// symlink with the error its backend spells that with.
	target, err := res.src.fsys.ReadLink(res.path)
	if err != nil {
		return "", fsutil.WrapPathErr("readlink", name, err)
	}
	return target, nil
}

// Open opens name for reading.
func (f *Fs) Open(name string) (vroot.File, error) {
	return f.OpenFile(name, os.O_RDONLY, 0)
}

// OpenFile opens name with the given flags.
//
// A directory comes back as a handle over the merged listing, whichever layers
// contributed to it. A regular file comes from the single layer it resolved to,
// read-only unless that layer is the top.
//
// Any flag asking for more than a read sends the open through the copy-up gate
// instead: the file is brought into the top, or created there, and the handle is
// onto the top's copy.
func (f *Fs) OpenFile(name string, flag int, perm fs.FileMode) (vroot.File, error) {
	if writeFlag(flag) {
		return f.openWrite(name, flag, perm)
	}

	f.state.mu.RLock()
	defer f.state.mu.RUnlock()

	key, res, err := f.locate(name, false)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	inner, err := res.src.fsys.OpenFile(res.path, flag, perm)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}

	if res.info.IsDir() {
		return f.track(name, key, newMergedDir(f, name, key, inner)), nil
	}
	if res.layer > 0 {
		// A lower is read as it stands and never written to: the handle would be
		// the one write that did not pass the copy-up gate. The error is dropped
		// rather than checked — NewReadOnlyFile only ever returns the one it is
		// handed, so that the open it wraps can be passed straight in.
		inner, _ = vroot.NewReadOnlyFile(inner, nil)
	}
	return f.track(name, key, inner), nil
}

// OpenRoot returns the overlay confined to the directory name, sharing the
// layers, the masking state and the lock with the overlay it was opened from
// rather than stacking anything of its own (§4.5).
//
// The result is a [vroot.Root] like its parent: paths in it are resolved against
// the sub-root, and neither ".." nor a symlink can address anything above it,
// even where the overlay it came from still could.
func (f *Fs) OpenRoot(name string) (*Fs, error) {
	f.state.mu.RLock()
	defer f.state.mu.RUnlock()

	base, res, err := f.locate(name, false)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	if !res.info.IsDir() {
		return nil, fsutil.WrapPathErr("open", name, syscall.ENOTDIR)
	}

	sub := *f
	sub.base = base
	sub.owner = false
	return &sub, nil
}

// track counts the handle out and hands back one that counts it back in when it
// is closed. name is the caller's own spelling, key the merged-view name the
// count is kept per.
func (f *Fs) track(name string, key string, file vroot.File) vroot.File {
	f.handles.acquire(key)
	return &trackedFile{File: file, name: name, key: key, handles: f.handles}
}

// writeFlag reports whether flag asks for more than a read. It mirrors vroot's
// internal openflag.WriteOp, which is internal to that module and so cannot be
// imported here.
func writeFlag(flag int) bool {
	return flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0
}
