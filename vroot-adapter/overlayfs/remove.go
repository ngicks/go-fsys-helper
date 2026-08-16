package overlayfs

import (
	"errors"
	"io/fs"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
)

// Remove deletes name: the file itself when it is a symlink, and an empty
// directory when it is one — empty in the merged view, whichever layers its
// entries come from.
//
// A name the layers below still show is whited out rather than merely dropped
// from the top, since dropping it would let the lower copy resurface.
func (f *Fs) Remove(name string) error {
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	return fsutil.WrapPathErr("remove", name, f.remove(name))
}

func (f *Fs) remove(name string) error {
	if namesSelf(name) {
		return syscall.EINVAL
	}
	full, err := f.resolve(name, true)
	if err != nil {
		return err
	}
	if full == "" {
		return syscall.EINVAL
	}
	res, err := f.state.lookup(full)
	if err != nil {
		return err
	}
	if err := f.removable(full); err != nil {
		return err
	}
	if res.info.IsDir() {
		entries, err := f.state.readDir(full)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return errdef.ENOTEMPTY
		}
	}
	return f.unlink(full, f.top().fsys.Remove)
}

// RemoveAll deletes name and everything under it, across every layer that
// contributes to it, and reports success for a name that is not there.
//
// One whiteout masks the whole lower subtree and one RemoveAll clears the top's,
// so a tree the two are mixed across costs the same as either alone.
func (f *Fs) RemoveAll(name string) error {
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	return fsutil.WrapPathErr("RemoveAll", name, f.removeAll(name))
}

func (f *Fs) removeAll(name string) error {
	if namesSelf(name) {
		return syscall.EINVAL
	}
	full, err := f.resolve(name, true)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	}
	if full == "" {
		return syscall.EINVAL
	}
	switch _, err := f.state.lookup(full); {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	}
	if err := f.removable(full); err != nil {
		return err
	}
	return f.unlink(full, f.top().fsys.RemoveAll)
}

// removable is the [Option.DisableOpenFileRemoval] gate: a name the overlay still
// has a handle out on cannot be removed, the way Windows refuses to unlink an
// open file. The count is kept per resolved merged-view name, which is what the
// handles were counted out under.
func (f *Fs) removable(full string) error {
	if f.disableOpenFileRemoval && f.handles.count(full) > 0 {
		return errSharingViolation
	}
	return nil
}

// unlink drops full from the top with remove and brings the masking around it in
// line: a name the layers below still show is whited out, and one they do not has
// its marks — and everything recorded under it — forgotten, so a later create
// over the name starts from nothing rather than from a mask describing a tree
// that is gone.
//
// The whiteout is recorded before the top gives the name up. The other order
// leaves a window, when the store write fails, in which the layers below have
// resurfaced under a name the caller was told is gone.
func (f *Fs) unlink(full string, remove func(path string) error) error {
	switch _, err := f.state.lookupFrom(full, 1); {
	case err == nil:
		if err := f.state.setWhiteout(full); err != nil {
			return err
		}
	case !errors.Is(err, fs.ErrNotExist):
		return err
	default:
		if err := f.state.clearSubtree(full); err != nil {
			return err
		}
	}
	if err := remove(f.topPath(full)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
