package synthfs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"sync"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/clock"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
)

// memBuf is the shared byte buffer behind a memView. Multiple handles opened
// from the same view share one buf, so writes through one handle are visible
// to subsequent reads through another.
type memBuf struct {
	clock clock.WallClock

	mu      sync.RWMutex
	content []byte
	mode    fs.FileMode
	modTime time.Time
}

func (b *memBuf) stat(name string) stat {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return stat{
		name:    name,
		size:    int64(len(b.content)),
		mode:    b.mode,
		modTime: b.modTime,
	}
}

func (b *memBuf) length() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.content)
}

func (b *memBuf) readAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("synthfs: negative offset")
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if off >= int64(len(b.content)) {
		return 0, io.EOF
	}
	n := copy(p, b.content[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (b *memBuf) writeAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("synthfs: negative offset: %w", syscall.EINVAL)
	}
	if off > math.MaxInt {
		return 0, fmt.Errorf("synthfs: offset overflows int: %w", syscall.EINVAL)
	}
	if int64(len(p)) > math.MaxInt64-off {
		return 0, fmt.Errorf("synthfs: offset+len overflows int64: %w", syscall.EINVAL)
	}
	if len(p) == 0 {
		return 0, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	end := int(off) + len(p)
	if end > len(b.content) {
		b.grow(end - len(b.content))
	}
	n := copy(b.content[int(off):], p)
	b.modTime = b.clock.Now()
	return n, nil
}

func (b *memBuf) truncate(size int64) error {
	if size < 0 {
		return syscall.EINVAL
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if grow := int(size) - len(b.content); grow > 0 {
		b.grow(grow)
	}
	b.content = b.content[:size:size]
	b.modTime = b.clock.Now()
	return nil
}

// grow extends content by `by` bytes. Caller holds b.mu (write).
func (b *memBuf) grow(by int) {
	if cap(b.content)-len(b.content) >= by {
		b.content = b.content[:len(b.content)+by]
		return
	}
	b.content = append(b.content, make([]byte, by)...)
}

// memHandle is one stateful handle into a [memBuf]. Multiple handles can
// share a buf and each has its own offset and flag set.
type memHandle struct {
	buf  *memBuf
	name string

	mu   sync.Mutex
	flag int
	off  int64
}

var _ vroot.File = (*memHandle)(nil)

func (h *memHandle) Name() string {
	return h.name
}

func (h *memHandle) Fd() uintptr {
	return ^uintptr(0)
}

func (h *memHandle) Close() error {
	// Tree-side handle wrapper drives the refcount decrement; the buf itself
	// has nothing to release.
	return nil
}

func (h *memHandle) Stat() (fs.FileInfo, error) {
	return h.buf.stat(path.Base(h.name)), nil
}

func (h *memHandle) Sync() error {
	// In-memory writes are durable as soon as the call returns.
	return nil
}

func (h *memHandle) Chmod(mode fs.FileMode) error {
	h.buf.mu.Lock()
	defer h.buf.mu.Unlock()
	h.buf.mode = mode & fs.ModePerm
	return nil
}

func (h *memHandle) Chown(uid, gid int) error {
	// memBuf doesn't track ownership; the tree node does.
	return nil
}

func (h *memHandle) Read(p []byte) (int, error) {
	if !openflag.Readable(h.flag) {
		return 0, fsutil.WrapPathErr("read", h.name, errdef.EBADF)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	n, err := h.buf.readAt(p, h.off)
	h.off += int64(n)
	if err == io.EOF {
		return n, err
	}
	if err != nil {
		return n, fsutil.WrapPathErr("read", h.name, err)
	}
	return n, nil
}

func (h *memHandle) ReadAt(p []byte, off int64) (int, error) {
	if !openflag.Readable(h.flag) {
		return 0, fsutil.WrapPathErr("readat", h.name, errdef.EBADF)
	}
	n, err := h.buf.readAt(p, off)
	if err == io.EOF {
		return n, err
	}
	if err != nil {
		return n, fsutil.WrapPathErr("readat", h.name, err)
	}
	return n, nil
}

func (h *memHandle) Write(p []byte) (int, error) {
	if !openflag.Writable(h.flag) {
		return 0, fsutil.WrapPathErr("write", h.name, errdef.EBADF)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.flag&os.O_APPEND != 0 {
		h.off = int64(h.buf.length())
	}
	n, err := h.buf.writeAt(p, h.off)
	h.off += int64(n)
	if err != nil {
		return n, fsutil.WrapPathErr("write", h.name, err)
	}
	return n, nil
}

func (h *memHandle) WriteAt(p []byte, off int64) (int, error) {
	if h.flag&os.O_APPEND != 0 {
		// POSIX rejects pwrite on O_APPEND fds.
		return 0, fsutil.WrapPathErr("writeat", h.name, syscall.EINVAL)
	}
	if !openflag.Writable(h.flag) {
		return 0, fsutil.WrapPathErr("writeat", h.name, errdef.EBADF)
	}
	n, err := h.buf.writeAt(p, off)
	if err != nil {
		return n, fsutil.WrapPathErr("writeat", h.name, err)
	}
	return n, nil
}

func (h *memHandle) WriteString(s string) (int, error) {
	return h.Write([]byte(s))
}

func (h *memHandle) Seek(offset int64, whence int) (int64, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += h.off
	case io.SeekEnd:
		offset += int64(h.buf.length())
	default:
		return 0, fsutil.WrapPathErr(
			"seek",
			h.name,
			fmt.Errorf("unknown whence %d: %w", whence, fs.ErrInvalid),
		)
	}
	if offset < 0 {
		return 0, fsutil.WrapPathErr(
			"seek",
			h.name,
			fmt.Errorf("negative offset %d: %w", offset, fs.ErrInvalid),
		)
	}
	h.off = offset
	return h.off, nil
}

func (h *memHandle) Truncate(size int64) error {
	if !openflag.Writable(h.flag) {
		return fsutil.WrapPathErr("truncate", h.name, errdef.EBADF)
	}
	if err := h.buf.truncate(size); err != nil {
		return fsutil.WrapPathErr("truncate", h.name, err)
	}
	return nil
}

func (h *memHandle) ReadDir(int) ([]fs.DirEntry, error) {
	return nil, fsutil.WrapPathErr("readdir", h.name, syscall.ENOTDIR)
}

func (h *memHandle) Readdir(int) ([]fs.FileInfo, error) {
	return nil, fsutil.WrapPathErr("readdir", h.name, syscall.ENOTDIR)
}

func (h *memHandle) Readdirnames(int) ([]string, error) {
	return nil, fsutil.WrapPathErr("readdir", h.name, syscall.ENOTDIR)
}
