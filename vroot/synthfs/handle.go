package synthfs

import (
	"io/fs"
	"sync"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
)

// fileHandle wraps a view-provided [vroot.File] with tree-aware bookkeeping:
//   - Name() reports the user-supplied path
//   - Chmod/Chown act on the tree node, not the view content
//   - Stat composes the node's mode/modTime with the view's live size
//   - Close decrements the node's refCount
//
// Writes are forwarded to the view; the modTime stays where the view writes it
// (memBuf bumps modTime on every write). For external views this means the
// modTime reflects the view's own clock rather than synthfs's, which is the
// right answer for read-only sources.
type fileHandle struct {
	st    *state
	node  *file
	name  string // user-facing, OS-form path
	flag  int
	inner vroot.File

	mu     sync.Mutex
	closed bool
}

var _ vroot.File = (*fileHandle)(nil)

func (h *fileHandle) Name() string {
	return h.name
}

func (h *fileHandle) Fd() uintptr {
	return ^uintptr(0)
}

func (h *fileHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil
	}
	h.closed = true
	h.st.mu.Lock()
	if h.node.refCount > 0 {
		h.node.refCount--
	}
	h.st.mu.Unlock()
	return h.inner.Close()
}

func (h *fileHandle) isClosed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

func (h *fileHandle) Read(p []byte) (int, error) {
	if h.isClosed() {
		return 0, fsutil.WrapPathErr("read", h.name, fs.ErrClosed)
	}
	if !openflag.Readable(h.flag) {
		return 0, fsutil.WrapPathErr("read", h.name, errdef.EBADF)
	}
	return h.inner.Read(p)
}

func (h *fileHandle) ReadAt(p []byte, off int64) (int, error) {
	if h.isClosed() {
		return 0, fsutil.WrapPathErr("readat", h.name, fs.ErrClosed)
	}
	if !openflag.Readable(h.flag) {
		return 0, fsutil.WrapPathErr("readat", h.name, errdef.EBADF)
	}
	return h.inner.ReadAt(p, off)
}

func (h *fileHandle) Write(p []byte) (int, error) {
	if h.isClosed() {
		return 0, fsutil.WrapPathErr("write", h.name, fs.ErrClosed)
	}
	if !openflag.Writable(h.flag) {
		return 0, fsutil.WrapPathErr("write", h.name, errdef.EBADF)
	}
	return h.inner.Write(p)
}

func (h *fileHandle) WriteAt(p []byte, off int64) (int, error) {
	if h.isClosed() {
		return 0, fsutil.WrapPathErr("writeat", h.name, fs.ErrClosed)
	}
	if !openflag.Writable(h.flag) {
		return 0, fsutil.WrapPathErr("writeat", h.name, errdef.EBADF)
	}
	return h.inner.WriteAt(p, off)
}

func (h *fileHandle) WriteString(s string) (int, error) {
	if h.isClosed() {
		return 0, fsutil.WrapPathErr("write", h.name, fs.ErrClosed)
	}
	if !openflag.Writable(h.flag) {
		return 0, fsutil.WrapPathErr("write", h.name, errdef.EBADF)
	}
	return h.inner.WriteString(s)
}

func (h *fileHandle) Truncate(size int64) error {
	if h.isClosed() {
		return fsutil.WrapPathErr("truncate", h.name, fs.ErrClosed)
	}
	if !openflag.Writable(h.flag) {
		return fsutil.WrapPathErr("truncate", h.name, errdef.EBADF)
	}
	return h.inner.Truncate(size)
}

func (h *fileHandle) Seek(offset int64, whence int) (int64, error) {
	if h.isClosed() {
		return 0, fsutil.WrapPathErr("seek", h.name, fs.ErrClosed)
	}
	return h.inner.Seek(offset, whence)
}

func (h *fileHandle) Sync() error {
	if h.isClosed() {
		return fsutil.WrapPathErr("sync", h.name, fs.ErrClosed)
	}
	return h.inner.Sync()
}

func (h *fileHandle) Stat() (fs.FileInfo, error) {
	h.st.mu.RLock()
	defer h.st.mu.RUnlock()
	s := h.node.toStat()
	info, err := h.node.view.Stat()
	if err != nil {
		return nil, fsutil.WrapPathErr("stat", h.name, err)
	}
	s.size = info.Size()
	return s, nil
}

func (h *fileHandle) Chmod(mode fs.FileMode) error {
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	chmodApply(&h.st.opt, &h.node.metadata, mode)
	return nil
}

func (h *fileHandle) Chown(uid, gid int) error {
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	h.node.uid = uid
	h.node.gid = gid
	return nil
}

func (h *fileHandle) ReadDir(int) ([]fs.DirEntry, error) {
	return nil, fsutil.WrapPathErr("readdir", h.name, syscall.ENOTDIR)
}

func (h *fileHandle) Readdir(int) ([]fs.FileInfo, error) {
	return nil, fsutil.WrapPathErr("readdir", h.name, syscall.ENOTDIR)
}

func (h *fileHandle) Readdirnames(int) ([]string, error) {
	return nil, fsutil.WrapPathErr("readdir", h.name, syscall.ENOTDIR)
}
