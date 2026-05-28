package synthfs

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// dirHandle is the [vroot.File] returned by Open on a directory. It exposes
// ReadDir/Readdir/Readdirnames over a cursor into the dir's ordered entries.
// Reads/writes are rejected with EISDIR.
type dirHandle struct {
	st   *state
	node *dir
	name string // user-facing, OS-form path

	mu     sync.Mutex
	closed bool
	cursor int
}

var _ vroot.File = (*dirHandle)(nil)

func (h *dirHandle) Name() string { return h.name }
func (h *dirHandle) Fd() uintptr  { return ^uintptr(0) }
func (h *dirHandle) Sync() error  { return nil }

func (h *dirHandle) Close() error {
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
	return nil
}

func (h *dirHandle) Stat() (fs.FileInfo, error) {
	h.st.mu.RLock()
	defer h.st.mu.RUnlock()
	return h.node.toStat(), nil
}

func (h *dirHandle) Read([]byte) (int, error) {
	return 0, fsutil.WrapPathErr("read", h.name, syscall.EISDIR)
}

func (h *dirHandle) ReadAt([]byte, int64) (int, error) {
	return 0, fsutil.WrapPathErr("readat", h.name, syscall.EISDIR)
}

func (h *dirHandle) Write([]byte) (int, error) {
	return 0, fsutil.WrapPathErr("write", h.name, syscall.EISDIR)
}

func (h *dirHandle) WriteAt([]byte, int64) (int, error) {
	return 0, fsutil.WrapPathErr("writeat", h.name, syscall.EISDIR)
}

func (h *dirHandle) WriteString(string) (int, error) {
	return 0, fsutil.WrapPathErr("write", h.name, syscall.EISDIR)
}

func (h *dirHandle) Truncate(int64) error {
	return fsutil.WrapPathErr("truncate", h.name, syscall.EISDIR)
}

// Seek mimics *os.File on a directory: only no-op rewinds/positioning to end
// of stream are accepted.
func (h *dirHandle) Seek(offset int64, whence int) (int64, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch whence {
	case io.SeekStart:
		if offset < 0 {
			return 0, fsutil.WrapPathErr("seek", h.name, fmt.Errorf("negative offset %d: %w", offset, fs.ErrInvalid))
		}
		h.cursor = 0
	case io.SeekCurrent:
		if offset != 0 {
			return 0, fsutil.WrapPathErr("seek", h.name, fs.ErrInvalid)
		}
	case io.SeekEnd:
		if offset > 0 {
			return 0, fsutil.WrapPathErr("seek", h.name, fmt.Errorf("positive offset %d: %w", offset, fs.ErrInvalid))
		}
		h.st.mu.RLock()
		h.cursor = h.node.ordered.Len()
		h.st.mu.RUnlock()
	default:
		return 0, fsutil.WrapPathErr("seek", h.name, fmt.Errorf("unknown whence %d: %w", whence, fs.ErrInvalid))
	}
	return 0, nil
}

// readEntries advances the cursor by up to n entries; n<=0 means "until end."
// Caller holds h.mu; the returned snapshot is a shallow copy safe to use
// outside the lock.
func (h *dirHandle) readEntries(n int) ([]node, error) {
	if h.closed {
		return nil, fsutil.WrapPathErr("readdir", h.name, fs.ErrClosed)
	}
	h.st.mu.RLock()
	defer h.st.mu.RUnlock()
	all := h.node.entriesSnapshot()
	if h.cursor >= len(all) {
		if n <= 0 {
			return nil, nil
		}
		return nil, io.EOF
	}
	end := len(all)
	if n > 0 && h.cursor+n < end {
		end = h.cursor + n
	}
	out := make([]node, end-h.cursor)
	copy(out, all[h.cursor:end])
	h.cursor = end
	return out, nil
}

func (h *dirHandle) ReadDir(n int) ([]fs.DirEntry, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	entries, err := h.readEntries(n)
	if err != nil {
		return nil, err
	}
	out := make([]fs.DirEntry, 0, len(entries))
	for _, nd := range entries {
		out = append(out, fs.FileInfoToDirEntry(nd.meta().toStat()))
	}
	return out, nil
}

func (h *dirHandle) Readdir(n int) ([]fs.FileInfo, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	entries, err := h.readEntries(n)
	if err != nil {
		return nil, err
	}
	out := make([]fs.FileInfo, 0, len(entries))
	for _, nd := range entries {
		out = append(out, nd.meta().toStat())
	}
	return out, nil
}

func (h *dirHandle) Readdirnames(n int) ([]string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	entries, err := h.readEntries(n)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, nd := range entries {
		out = append(out, filepath.Base(nd.meta().name))
	}
	return out, nil
}

func (h *dirHandle) Chmod(mode fs.FileMode) error {
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	chmodApply(&h.st.opt, &h.node.metadata, mode)
	return nil
}

func (h *dirHandle) Chown(uid, gid int) error {
	h.st.mu.Lock()
	defer h.st.mu.Unlock()
	h.node.uid = uid
	h.node.gid = gid
	return nil
}
