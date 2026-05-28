package synthfs

import (
	"io/fs"
	"path/filepath"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
)

// Stat follows symlinks and returns the metadata of the target node.
func (r *Root) Stat(name string) (fs.FileInfo, error) {
	r.st.mu.RLock()
	defer r.st.mu.RUnlock()
	res, err := r.findNode(name, true, "stat")
	if err != nil {
		return nil, err
	}
	return r.statNode(res.node)
}

// Lstat does not follow the final symlink; otherwise it behaves like [Stat].
func (r *Root) Lstat(name string) (fs.FileInfo, error) {
	r.st.mu.RLock()
	defer r.st.mu.RUnlock()
	res, err := r.findNode(name, false, "lstat")
	if err != nil {
		return nil, err
	}
	return r.statNode(res.node)
}

// ReadLink returns the target of the symlink at name without following it.
func (r *Root) ReadLink(name string) (string, error) {
	r.st.mu.RLock()
	defer r.st.mu.RUnlock()
	res, err := r.findNode(name, false, "readlink")
	if err != nil {
		return "", err
	}
	sym, ok := res.node.(*symlink)
	if !ok {
		return "", toPathErr("readlink", name, errdef.EBADF)
	}
	return filepath.FromSlash(sym.target), nil
}

// statNode composes the node's metadata into an [fs.FileInfo]. For *file the
// authoritative size comes from the view. Caller must hold state.mu (read).
func (r *Root) statNode(n node) (fs.FileInfo, error) {
	if f, ok := n.(*file); ok {
		s, err := statFromFileView(&f.metadata, f.view)
		if err != nil {
			return nil, fsutil.WrapPathErr("stat", f.name, err)
		}
		return s, nil
	}
	return n.meta().toStat(), nil
}
