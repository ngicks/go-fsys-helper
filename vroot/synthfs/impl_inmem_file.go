package synthfs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/clock"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
)

var _ vroot.File = (*memFileHandle)(nil)

type memFileHandle struct {
	mu   sync.Mutex
	file *memFile
	path string
	off  int64
	flag int
}

func newMemFileHandle(file *memFile, path string, flag int) *memFileHandle {
	return &memFileHandle{
		file: file,
		path: path,
		flag: flag,
	}
}

func (f *memFileHandle) Close() error {
	// close is handled by wrapper
	return nil
}

func (f *memFileHandle) Name() string {
	return f.path
}

func (f *memFileHandle) Read(p []byte) (n int, err error) {
	if !openflag.Readable(f.flag) {
		return 0, fsutil.WrapPathErr("read", f.path, syscall.EBADF)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	n, err = f.file.ReadAt(p, f.off)
	if err != nil && err != io.EOF {
		err = fsutil.WrapPathErr("read", f.path, err)
	}
	f.off += int64(n)
	return
}

func (f *memFileHandle) ReadAt(p []byte, off int64) (n int, err error) {
	if !openflag.Readable(f.flag) {
		return 0, fsutil.WrapPathErr("readat", f.path, syscall.EBADF)
	}
	n, err = f.file.ReadAt(p, off)
	if err != nil && err != io.EOF {
		err = fsutil.WrapPathErr("readat", f.path, err)
	}
	return
}

func (f *memFileHandle) ReadDir(count int) ([]fs.DirEntry, error) {
	return nil, fsutil.WrapPathErr("readdir", f.path, syscall.ENOTDIR)
}

func (f *memFileHandle) Readdir(count int) ([]fs.FileInfo, error) {
	return nil, fsutil.WrapPathErr("readdir", f.path, syscall.ENOTDIR)
}

func (f *memFileHandle) Readdirnames(n int) ([]string, error) {
	return nil, fsutil.WrapPathErr("readdir", f.path, syscall.ENOTDIR)
}

func (f *memFileHandle) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch whence {
	default:
		return 0, fsutil.WrapPathErr("seek", f.path, fmt.Errorf("invalid whence: %d", whence))
	case io.SeekStart:
	case io.SeekCurrent:
		offset += f.off
	case io.SeekEnd:
		offset += int64(f.file.Len())
	}

	if offset < 0 {
		return 0, fsutil.WrapPathErr("seek", f.path, fmt.Errorf("negative position"))
	}

	f.off = offset

	return f.off, nil
}

func (f *memFileHandle) Stat() (fs.FileInfo, error) {
	return f.file.stat(f.path), nil
}

func (f *memFileHandle) Sync() error {
	// always synced.
	return nil
}

func (f *memFileHandle) Truncate(size int64) error {
	if !openflag.Writable(f.flag) {
		return fsutil.WrapPathErr("truncate", f.path, syscall.EBADF)
	}
	err := f.file.Truncate(size)
	if err != nil {
		return fsutil.WrapPathErr("truncate", f.path, err)
	}
	return nil
}

func (f *memFileHandle) Write(p []byte) (n int, err error) {
	if !openflag.Writable(f.flag) {
		return 0, fsutil.WrapPathErr("write", f.path, syscall.EBADF)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.flag&os.O_APPEND != 0 {
		f.off = int64(f.file.Len())
	}
	n, err = f.file.WriteAt(p, f.off)
	if err != nil {
		err = fsutil.WrapPathErr("write", f.path, err)
	}
	f.off += int64(n)
	return
}

func (f *memFileHandle) WriteAt(p []byte, off int64) (n int, err error) {
	if f.flag&os.O_APPEND != 0 {
		return 0, fsutil.WrapPathErr("writeat", f.path, syscall.EINVAL)
	}
	if !openflag.Writable(f.flag) {
		return 0, fsutil.WrapPathErr("writeat", f.path, syscall.EBADF)
	}
	n, err = f.file.WriteAt(p, off)
	if err != nil {
		err = fsutil.WrapPathErr("writeat", f.path, err)
	}
	return
}

func (f *memFileHandle) WriteString(s string) (ret int, err error) {
	return f.Write([]byte(s))
}

// vroot.File interface methods

func (f *memFileHandle) Chmod(mode fs.FileMode) error {
	f.file.mu.Lock()
	defer f.file.mu.Unlock()
	f.file.mode = chmodMask & mode
	return nil
}

func (f *memFileHandle) Chown(uid, gid int) error {
	// No-op for in-memory files
	return nil
}

func (f *memFileHandle) Fd() uintptr {
	return ^(uintptr(0))
}

var (
	_ io.ReaderAt = (*memFile)(nil)
	_ io.WriterAt = (*memFile)(nil)
)

type memFile struct {
	clock clock.WallClock

	mu      sync.RWMutex
	mode    fs.FileMode
	modTime time.Time
	content []byte
}

func newMemFile(mode fs.FileMode, clock clock.WallClock) *memFile {
	return &memFile{
		clock:   clock,
		mode:    mode & chmodMask,
		modTime: clock.Now(),
	}
}

func (f *memFile) Len() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.content)
}

func (f *memFile) stat(name string) stat {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return stat{f.mode, f.modTime, name, int64(len(f.content))}
}

func (f *memFile) Truncate(size int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if size < 0 {
		return syscall.EINVAL
	}
	diff := size - int64(len(f.content))
	if diff > 0 {
		f.grow(int(diff))
	}
	f.content = f.content[:size:size] // release unused portion
	f.modTime = f.clock.Now()
	return nil
}

// ReadAt implements io.ReaderAt.
func (f *memFile) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, errors.New("negative offset")
	}

	// only read lock,
	// no state changes to conform interface.

	f.mu.RLock()
	defer f.mu.RUnlock()

	if off >= int64(len(f.content)) {
		return 0, io.EOF
	}

	n = copy(p, f.content[off:])
	if n < len(p) {
		err = io.EOF
	}
	return
}

// WriteAt implements io.WriterAt.
func (f *memFile) WriteAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, fmt.Errorf("%w: negative offset", syscall.EINVAL)
	}
	if off > math.MaxInt {
		return 0, fmt.Errorf("%w: off overflows max int: %d > %d", syscall.EINVAL, off, math.MaxInt)
	}
	if off+int64(len(p)) < off {
		return 0, fmt.Errorf("%w: off + len(p) overflows int64", syscall.EINVAL)
	}
	if len(p) == 0 {
		// no-op
		return 0, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	growth := int(off) + len(p) - len(f.content)
	if growth > 0 {
		f.grow(growth)
	}
	n = copy(f.content[int(off):], p)
	f.modTime = f.clock.Now()
	return
}

func (f *memFile) grow(growth int) {
	if cap(f.content)-len(f.content) >= growth {
		f.content = f.content[:len(f.content)+growth]
	} else {
		f.content = append(f.content, make([]byte, growth)...)
	}
}
