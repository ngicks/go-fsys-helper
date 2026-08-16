package synthfs

import (
	"errors"
	"io/fs"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
)

func (r *Root) Rename(oldname, newname string) error {
	r.st.mu.Lock()
	defer r.st.mu.Unlock()

	oldParent, oldBase, err := r.st.resolveParent(r.boundary, r.boundary, oldname, "rename")
	if err != nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, errors.Unwrap(err))
	}
	target := oldParent.lookup(oldBase)
	if target == nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, fs.ErrNotExist)
	}

	newParent, newBase, err := r.st.resolveParent(r.boundary, r.boundary, newname, "rename")
	if err != nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, errors.Unwrap(err))
	}
	if existing := newParent.lookup(newBase); existing != nil {
		// POSIX rename replaces; emulate that for files. Refuse to overwrite
		// a non-empty directory to keep the operation safe under any platform.
		if d, ok := existing.(*dir); ok && d.ordered.Len() > 0 {
			return fsutil.WrapLinkErr("rename", oldname, newname, errdef.ENOTEMPTY)
		}
		newParent.removeEntry(newBase)
		if f, ok := existing.(*file); ok {
			_ = f.view.Close()
		}
	}

	oldParent.removeEntry(oldBase)
	target.meta().name = newBase
	if d, ok := target.(*dir); ok {
		d.parent = newParent
	}
	newParent.addEntry(newBase, target)
	return nil
}
