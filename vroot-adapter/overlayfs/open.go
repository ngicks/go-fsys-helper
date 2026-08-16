package overlayfs

import (
	"errors"
	"io/fs"
	"os"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// Create creates or truncates name in the top and opens it for reading and
// writing, as [os.Create] does.
func (f *Fs) Create(name string) (vroot.File, error) {
	return f.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

// openWrite is the write half of [Fs.OpenFile]: the file is brought into the top,
// or created there, and the handle is onto the top's copy.
func (f *Fs) openWrite(name string, flag int, perm fs.FileMode) (vroot.File, error) {
	f.state.mu.Lock()
	defer f.state.mu.Unlock()

	creating := flag&os.O_CREATE != 0
	full, err := f.resolve(name, false)
	if err != nil {
		// A create names what may not be there yet, so the walk only has to reach
		// the parent and the final component is taken as written.
		if !creating || !errors.Is(err, fs.ErrNotExist) {
			return nil, fsutil.WrapPathErr("open", name, err)
		}
		full, err = f.resolve(name, true)
		if err != nil {
			return nil, fsutil.WrapPathErr("open", name, err)
		}
	}

	masked := false
	res, err := f.state.lookup(full)
	switch {
	case err == nil && res.info.IsDir():
		return nil, fsutil.WrapPathErr("open", name, syscall.EISDIR)
	case err == nil:
		if creating && flag&os.O_EXCL != 0 {
			return nil, fsutil.WrapPathErr("open", name, fs.ErrExist)
		}
		// O_TRUNC copies the data up only to drop it again. Copying anyway keeps
		// one gate for every write instead of a second path that has to get the
		// mode, the times and the parents right on its own.
		if err := f.copyUp(full); err != nil {
			return nil, fsutil.WrapPathErr("open", name, err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fsutil.WrapPathErr("open", name, err)
	case !creating:
		return nil, fsutil.WrapPathErr("open", name, fs.ErrNotExist)
	default:
		if err := f.ensureParent(full); err != nil {
			return nil, fsutil.WrapPathErr("open", name, err)
		}
		masked, _ = f.state.marks(0, full)
	}

	inner, err := f.top().fsys.OpenFile(f.topPath(full), flag, perm)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	// Lifted after the file exists, not before: a whiteout dropped for an open
	// that then failed would have brought back the lower content the removal
	// masked, under a name the caller was told nothing was created at.
	if masked {
		if err := f.state.clearWhiteout(full); err != nil {
			_ = inner.Close()
			return nil, fsutil.WrapPathErr("open", name, err)
		}
	}
	return f.track(name, full, inner), nil
}
