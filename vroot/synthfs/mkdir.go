package synthfs

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot/internal/paths"
)

func (r *Root) Mkdir(name string, perm fs.FileMode) error {
	return r.withWriteLock(func() error {
		return r.mkdirLocked(name, perm)
	})
}

func (r *Root) MkdirAll(name string, perm fs.FileMode) error {
	slash, err := normalize(name)
	if err != nil {
		return toPathErr("mkdir", name, err)
	}
	if slash == "." {
		return nil
	}
	r.st.mu.Lock()
	defer r.st.mu.Unlock()
	for p := range paths.PathFromHead(filepath.FromSlash(slash)) {
		if err := r.mkdirLocked(p, perm); err != nil {
			if !errors.Is(err, fs.ErrExist) {
				return err
			}
			// Existing path is fine for MkdirAll, as long as it's a dir.
			res, lerr := r.findNode(p, true, "mkdir")
			if lerr != nil {
				return lerr
			}
			if _, ok := res.node.(*dir); !ok {
				return toPathErr("mkdir", p, errdef.ENOTEMPTY)
			}
		}
	}
	return nil
}

// mkdirLocked is the body of [Mkdir]; callers must hold state.mu (write).
func (r *Root) mkdirLocked(name string, perm fs.FileMode) error {
	parent, base, err := r.st.resolveParent(r.boundary, r.boundary, name, "mkdir")
	if err != nil {
		return err
	}
	if base == "" || base == "." || base == ".." {
		return toPathErr("mkdir", name, fs.ErrInvalid)
	}
	if parent.lookup(base) != nil {
		return toPathErr("mkdir", name, fs.ErrExist)
	}
	nd := newDir(base, perm&^r.st.opt.umask, r.st.opt.clock.Now(), parent)
	parent.addEntry(base, nd)
	return nil
}
