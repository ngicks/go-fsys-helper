package overlayfs

import (
	"errors"
	"io/fs"

	"github.com/ngicks/go-fsys-helper/fsutil"
)

// Link creates newname as a hard link to oldname, without following oldname when
// it is a symlink.
//
// A hard link is two names for one file, which holds only inside one fs, so a
// lower-backed source is copied up and the link is made in the top — what the
// reference does as well (overlay.rs:2286-2289).
func (f *Fs) Link(oldname string, newname string) error {
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	return fsutil.WrapLinkErr("link", oldname, newname, f.link(oldname, newname))
}

func (f *Fs) link(oldname string, newname string) error {
	oldFull, err := f.resolve(oldname, true)
	if err != nil {
		return err
	}
	if _, err := f.state.lookup(oldFull); err != nil {
		return err
	}
	newFull, err := f.resolve(newname, true)
	if err != nil {
		return err
	}
	switch _, err := f.state.lookup(newFull); {
	case err == nil:
		return fs.ErrExist
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	if err := f.copyUp(oldFull); err != nil {
		return err
	}
	if err := f.ensureParent(newFull); err != nil {
		return err
	}
	if err := f.top().fsys.Link(f.topPath(oldFull), f.topPath(newFull)); err != nil {
		return err
	}
	return f.unmask(newFull)
}

// Symlink creates newname as a symlink holding oldname.
//
// oldname is stored as it was given and nothing is copied up for it: a symlink
// names a path, and what that path resolves to is the merged view's business
// when the link is read rather than this call's.
func (f *Fs) Symlink(oldname string, newname string) error {
	f.state.mu.Lock()
	defer f.state.mu.Unlock()
	return fsutil.WrapLinkErr("symlink", oldname, newname, f.symlink(oldname, newname))
}

func (f *Fs) symlink(oldname string, newname string) error {
	newFull, err := f.resolve(newname, true)
	if err != nil {
		return err
	}
	switch _, err := f.state.lookup(newFull); {
	case err == nil:
		return fs.ErrExist
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	if err := f.ensureParent(newFull); err != nil {
		return err
	}
	if err := f.top().fsys.Symlink(oldname, f.topPath(newFull)); err != nil {
		return err
	}
	return f.unmask(newFull)
}
