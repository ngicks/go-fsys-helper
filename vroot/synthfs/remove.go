package synthfs

import (
	"io/fs"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
)

func (r *Root) Remove(name string) error {
	return r.withWriteLock(func() error {
		parent, base, err := r.st.resolveParent(r.boundary, r.boundary, name, "remove")
		if err != nil {
			return err
		}
		target := parent.lookup(base)
		if target == nil {
			return toPathErr("remove", name, fs.ErrNotExist)
		}
		if d, ok := target.(*dir); ok && d.ordered.Len() > 0 {
			return toPathErr("remove", name, errdef.ENOTEMPTY)
		}
		if r.st.opt.disableOpenFileRemoval && target.meta().refCount > 0 {
			return toPathErr("remove", name, errSharingViolation)
		}
		parent.removeEntry(base)
		if f, ok := target.(*file); ok {
			_ = f.view.Close()
		}
		return nil
	})
}

func (r *Root) RemoveAll(name string) error {
	// Match os.RemoveAll: a trailing "." returns EINVAL before any filepath
	// rewrite can hide it.
	if endsWithDot(name) {
		return toPathErr("RemoveAll", name, syscall.EINVAL)
	}
	r.st.mu.Lock()
	defer r.st.mu.Unlock()
	return r.removeAllLocked(name)
}

func (r *Root) removeAllLocked(name string) error {
	slash, err := normalize(name)
	if err != nil {
		return toPathErr("RemoveAll", name, err)
	}
	if slash == "." {
		return toPathErr("RemoveAll", ".", syscall.EINVAL)
	}
	parent, base, err := r.st.resolveParent(r.boundary, r.boundary, name, "RemoveAll")
	if err != nil {
		return err
	}
	target := parent.lookup(base)
	if target == nil {
		// Mirror os.RemoveAll: a missing path is success.
		return nil
	}
	r.removeSubtree(target)
	parent.removeEntry(base)
	return nil
}

// removeSubtree closes any FileView attached to target or its descendants and
// unlinks all children. Caller holds state.mu (write).
func (r *Root) removeSubtree(n node) {
	switch nd := n.(type) {
	case *dir:
		for _, child := range nd.entriesSnapshot() {
			r.removeSubtree(child)
		}
		nd.ordered.Init()
		for k := range nd.index {
			delete(nd.index, k)
		}
	case *file:
		_ = nd.view.Close()
	}
}
