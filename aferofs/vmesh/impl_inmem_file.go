package vmesh

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

	"github.com/ngicks/go-fsys-helper/aferofs/clock"
	"github.com/ngicks/go-fsys-helper/aferofs/internal/errdef"
	"github.com/spf13/afero"
)

var _ afero.File = (*memFileHandle)(nil)

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
	if !flagReadable(f.flag) {
		return 0, errdef.ReadBadf(f.path)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	n, err = f.file.ReadAt(p, f.off)
	err = wrapErr("read", f.path, err)
	f.off += int64(n)
	return
}

func (f *memFileHandle) ReadAt(p []byte, off int64) (n int, err error) {
	if !flagReadable(f.flag) {
		return 0, errdef.ReadAtBadf(f.path)
	}
	return f.file.ReadAt(p, off)
}

func (f *memFileHandle) Readdir(count int) ([]fs.FileInfo, error) {
	return []fs.FileInfo{}, errdef.ReaddirNotADir(f.path)
}

func (f *memFileHandle) Readdirnames(n int) ([]string, error) {
	return []string{}, errdef.ReaddirNotADir(f.path)
}

func (f *memFileHandle) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch whence {
	default:
		return 0, errdef.SeekInval(f.path, fmt.Sprintf("unknown whence: %d", whence))
	case io.SeekStart:
	case io.SeekCurrent:
		offset += f.off
	case io.SeekEnd:
		offset += int64(f.file.Len())
	}

	if offset < 0 {
		return 0, errdef.SeekInval(f.path, "negative offset")
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

// Truncate implements afero.File.
func (f *memFileHandle) Truncate(size int64) error {
	if !flagWritable(f.flag) {
		return errdef.TruncateBadf(f.path)
	}
	err := f.file.Truncate(size)
	return wrapErr("truncate", f.path, err)
}

func (f *memFileHandle) Write(p []byte) (n int, err error) {
	if !flagWritable(f.flag) {
		return 0, errdef.WriteBadf(f.path)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.flag&os.O_APPEND != 0 {
		f.off = int64(f.file.Len())
	}
	n, err = f.file.WriteAt(p, f.off)
	err = wrapErr("write", f.path, err)
	f.off += int64(n)
	return
}

func (f *memFileHandle) WriteAt(p []byte, off int64) (n int, err error) {
	if f.flag&os.O_APPEND != 0 {
		return 0, errdef.WriteAtInAppendMode(f.path)
	}
	if !flagWritable(f.flag) {
		return 0, errdef.WriteAtBadf(f.path)
	}
	n, err = f.file.WriteAt(p, off)
	err = wrapErr("writeat", f.path, err)
	return
}

func (f *memFileHandle) WriteString(s string) (ret int, err error) {
	if !flagWritable(f.flag) {
		return 0, errdef.WriteAtBadf(f.path)
	}
	ret, err = f.Write([]byte(s))
	err = wrapErr("write", f.path, err)
	return
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
		mode:    mode & (fs.ModeType | fs.ModePerm),
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
		// TODO: prevent this over allocation?
		f.content = append(f.content, make([]byte, growth)...)
	}
}
