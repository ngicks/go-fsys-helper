package synthfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ngicks/go-fsys-helper/fsutil"
)

func (r *Root) Symlink(oldname, newname string) error {
	r.st.mu.Lock()
	defer r.st.mu.Unlock()
	parent, base, err := r.st.resolveParent(r.boundary, r.boundary, newname, "symlink")
	if err != nil {
		return fsutil.WrapLinkErr("symlink", oldname, newname, errors.Unwrap(err))
	}
	if parent.lookup(base) != nil {
		return fsutil.WrapLinkErr("symlink", oldname, newname, fs.ErrExist)
	}
	parent.addEntry(base, &symlink{
		metadata: metadata{
			name:    base,
			mode:    fs.ModeSymlink | 0o777,
			modTime: r.st.opt.clock.Now(),
		},
		target: filepath.ToSlash(oldname),
	})
	return nil
}

func (r *Root) Link(oldname, newname string) error {
	r.st.mu.Lock()
	defer r.st.mu.Unlock()

	srcRes, err := r.findNode(oldname, true, "link")
	if err != nil {
		return fsutil.WrapLinkErr("link", oldname, newname, errors.Unwrap(err))
	}
	srcFile, ok := srcRes.node.(*file)
	if !ok {
		return fsutil.WrapLinkErr("link", oldname, newname, os.ErrInvalid)
	}

	parent, base, err := r.st.resolveParent(r.boundary, r.boundary, newname, "link")
	if err != nil {
		return fsutil.WrapLinkErr("link", oldname, newname, errors.Unwrap(err))
	}
	if parent.lookup(base) != nil {
		return fsutil.WrapLinkErr("link", oldname, newname, fs.ErrExist)
	}
	// Hard link: share the same FileView so writes through either name reflect
	// on the other. Mode/uid/gid are copied at link time.
	parent.addEntry(base, &file{
		metadata: metadata{
			name:    base,
			mode:    srcFile.mode,
			modTime: srcFile.modTime,
			uid:     srcFile.uid,
			gid:     srcFile.gid,
		},
		view: srcFile.view,
	})
	return nil
}
