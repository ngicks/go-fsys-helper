package synthfs

import (
	"errors"
	"io/fs"
	"syscall"

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

	// Source == destination: a no-op success (POSIX rename(2) of a path onto
	// itself returns 0). Returning here also keeps the dir-subtree guard below
	// from mis-firing on Rename("a", "a").
	if newParent == oldParent && newBase == oldBase {
		return nil
	}

	// Reject moving a directory into its own subtree (e.g. Rename("a","a/b/c")).
	// Re-parenting a dir under a node inside the moved subtree detaches the
	// parent chain into a cycle: subsequent ".." walks would never reach the
	// boundary and could spin (the ELOOP cap counts only symlink hops, not
	// parent hops). POSIX returns EINVAL.
	if d, ok := target.(*dir); ok && r.isAncestorOrSelf(d, newParent) {
		return fsutil.WrapLinkErr("rename", oldname, newname, syscall.EINVAL)
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

// isAncestorOrSelf reports whether anc is start or one of its ancestors, walking
// the parent chain up to (and including) the Root's confinement boundary. Used
// to detect a rename that would move a directory into its own subtree. Caller
// holds state.mu.
func (r *Root) isAncestorOrSelf(anc, start *dir) bool {
	for cur := start; cur != nil; cur = cur.parent {
		if cur == anc {
			return true
		}
		if cur == r.boundary {
			break
		}
	}
	return false
}
