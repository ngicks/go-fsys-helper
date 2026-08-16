package synthfs

import (
	"io/fs"
	"os"

	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
)

func (r *Root) Create(name string) (vroot.File, error) {
	return r.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

func (r *Root) Open(name string) (vroot.File, error) {
	return r.OpenFile(name, os.O_RDONLY, 0)
}

func (r *Root) OpenFile(name string, flag int, perm fs.FileMode) (vroot.File, error) {
	r.st.mu.Lock()
	defer r.st.mu.Unlock()

	if flag&os.O_CREATE != 0 {
		parent, base, err := r.st.resolveParent(r.boundary, r.boundary, name, "open")
		if err != nil {
			return nil, err
		}
		if existing := parent.lookup(base); existing != nil {
			if flag&os.O_EXCL != 0 {
				return nil, toPathErr("open", name, fs.ErrExist)
			}
			// Fall through to opening the existing entry below.
		} else {
			view, err := r.st.opt.allocator.Allocate(base, perm&^r.st.opt.umask)
			if err != nil {
				return nil, toPathErr("open", name, err)
			}
			nf := &file{
				metadata: metadata{
					name:    base,
					mode:    perm &^ r.st.opt.umask,
					modTime: r.st.opt.clock.Now(),
				},
				view: view,
			}
			parent.addEntry(base, nf)
			return r.openFileNode(nf, name, flag)
		}
	}

	res, err := r.findNode(name, true, "open")
	if err != nil {
		return nil, err
	}
	switch nd := res.node.(type) {
	case *dir:
		if openflag.WriteOp(flag) {
			return nil, toPathErr("open", name, errdef.EBADF)
		}
		nd.refCount++
		return &dirHandle{st: r.st, node: nd, name: r.userPath(name)}, nil
	case *file:
		h, err := r.openFileNode(nd, name, flag)
		if err != nil {
			return nil, err
		}
		if flag&os.O_TRUNC != 0 && openflag.Writable(flag) {
			// Truncate through the open handle so aliased views (e.g. hard
			// links) see the same shrink. Re-Allocate would desynchronize.
			if terr := h.Truncate(0); terr != nil {
				_ = h.Close()
				return nil, terr
			}
		}
		return h, nil
	case *symlink:
		// Followed-last resolved to a symlink with no further target — *os.File
		// fails the same way.
		return nil, toPathErr("open", name, errdef.EBADF)
	}
	return nil, toPathErr("open", name, errInternalNodeType)
}

// openFileNode opens the view and wraps it in a fileHandle, bumping refCount
// on success. Caller holds state.mu (write).
func (r *Root) openFileNode(nd *file, userName string, flag int) (vroot.File, error) {
	inner, err := nd.view.Open(flag)
	if err != nil {
		return nil, toPathErr("open", userName, err)
	}
	nd.refCount++
	return &fileHandle{
		st:    r.st,
		node:  nd,
		name:  r.userPath(userName),
		flag:  flag,
		inner: inner,
	}, nil
}
