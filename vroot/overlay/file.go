package overlay

import (
	"fmt"
	"io"
	"io/fs"
	"math"
	"sync"
	"syscall"

	"github.com/ngicks/go-common/serr"
	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

var _ vroot.File = (*overlayFile)(nil)

// overlayFile wraps a layersFile for read-only access. Files opened with write flags
// bypass overlayFile and are immediately copied to the top layer.
type overlayFile struct {
	mu         sync.RWMutex
	name       string
	isDir      bool
	cursor     int
	closed     bool
	top        vroot.File
	layersFile *layersFile
}

// newOverlayFile creates a new overlay file for read-only access
func newOverlayFile(name string, idDir bool, top vroot.File, layersFile *layersFile) *overlayFile {
	return &overlayFile{
		name:       name,
		isDir:      idDir,
		top:        top,
		layersFile: layersFile,
	}
}

func (f *overlayFile) topFile() vroot.File {
	if f.top != nil {
		return f.top
	}
	return f.layersFile.topFile()
}

func (f *overlayFile) Name() string {
	return f.name
}

func (f *overlayFile) Fd() uintptr {
	return f.topFile().Fd()
}

func (f *overlayFile) Read(b []byte) (n int, err error) {
	return f.topFile().Read(b)
}

func (f *overlayFile) ReadAt(b []byte, off int64) (n int, err error) {
	return f.topFile().ReadAt(b, off)
}

func (f *overlayFile) checkClosed(op string) error {
	if f.closed {
		return fsutil.WrapPathErr(op, f.name, fs.ErrClosed)
	}
	return nil
}

func (f *overlayFile) Readdir(n int) ([]fs.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.checkClosed("readdir"); err != nil {
		// for overly sensitive caller, return empty slice.
		return []fs.FileInfo{}, err
	}

	if !f.isDir {
		return nil, &fs.PathError{Op: "readdir", Path: f.name, Err: syscall.ENOTDIR}
	}

	entries, err := f.layersFile.readDir(f.top)
	if err != nil {
		return nil, &fs.PathError{Op: "readdir", Path: f.name, Err: err}
	}

	if f.cursor >= len(entries) {
		if n <= 0 {
			return nil, nil
		}
		return nil, io.EOF
	}

	if n <= 0 {
		n = len(entries) - f.cursor
	}

	out := make([]fs.FileInfo, min(n, len(entries)-f.cursor))
	for i := range out {
		out[i] = entries[f.cursor]
		f.cursor++
	}

	return out, nil
}

func (f *overlayFile) ReadDir(n int) ([]fs.DirEntry, error) {
	entries, err := f.Readdir(n)
	if err != nil {
		return []fs.DirEntry{}, err
	}
	out := make([]fs.DirEntry, len(entries))
	for i, info := range entries {
		out[i] = fs.FileInfoToDirEntry(info)
	}
	return out, nil
}

// Readdirnames reads directory names
func (f *overlayFile) Readdirnames(n int) (names []string, err error) {
	entries, err := f.Readdir(n)
	if err != nil {
		return nil, err
	}

	out := make([]string, len(entries))
	for i, entry := range entries {
		out[i] = entry.Name()
	}
	return out, nil
}

// Seek sets the file offset
func (f *overlayFile) Seek(offset int64, whence int) (ret int64, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if err := f.checkClosed("seek"); err != nil {
		return 0, err
	}

	if !f.isDir {
		return f.topFile().Seek(offset, whence)
	}

	// mimicking *os.File behavior, reset anyway unless io.SeekEnd and 0 is set.
	f.cursor = 0
	f.layersFile.clearDirEnt()

	// lseek(3) on directory is not totally defined as far as I know.
	//
	// https://man7.org/linux/man-pages/man2/lseek.2.html
	// https://stackoverflow.com/questions/65911066/what-does-lseek-mean-for-a-directory-file-descriptor
	//
	// on windows Seek calls SetFilePointerEx. Does it work on handle for folder? I'm zero sure.
	//
	// So here place a simple rule.

	switch whence {
	default:
		return 0, fsutil.WrapPathErr("seek", f.name, fmt.Errorf("unknown whence %d: %w", whence, fs.ErrInvalid))
	case io.SeekStart:
		if offset < 0 {
			return 0, fsutil.WrapPathErr("seek", f.name, fmt.Errorf("negative offset %d: %w", whence, fs.ErrInvalid))
		}
	case io.SeekCurrent:
		if offset != 0 {
			return 0, fsutil.WrapPathErr("seek", f.name, fs.ErrInvalid)
		}
	case io.SeekEnd:
		if offset > 0 {
			return 0, fsutil.WrapPathErr("seek", f.name, fmt.Errorf("positive offset %d: %w", whence, fs.ErrInvalid))
		}
		f.cursor = math.MaxInt
	}

	return 0, nil
}

func (f *overlayFile) Stat() (fs.FileInfo, error) {
	return f.topFile().Stat()
}

// Close closes all files
func (f *overlayFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.closed = true

	errs := make([]serr.PrefixErr, len(f.layersFile.files))
	for i, file := range f.layersFile.files {
		errs[i] = serr.PrefixErr{
			P: fmt.Sprintf("file %d: ", i),
			E: file.Close(),
		}
	}

	return serr.GatherPrefixed(errs)
}

func (f *overlayFile) writeErr(op string) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if err := f.checkClosed(op); err != nil {
		return err
	}

	if f.isDir {
		return &fs.PathError{Op: op, Path: f.name, Err: syscall.EBADF}
	}
	return &fs.PathError{Op: op, Path: f.name, Err: syscall.EPERM}
}

func (f *overlayFile) Write(b []byte) (n int, err error) {
	return 0, f.writeErr("write")
}

func (f *overlayFile) WriteAt(b []byte, off int64) (n int, err error) {
	return 0, f.writeErr("writeat")
}

func (f *overlayFile) WriteString(s string) (n int, err error) {
	return 0, f.writeErr("write")
}

func (f *overlayFile) Truncate(size int64) error {
	return f.writeErr("truncate")
}

func (f *overlayFile) Sync() error {
	return f.writeErr("sync")
}

func (f *overlayFile) Chmod(mode fs.FileMode) error {
	return f.writeErr("chmod")
}

func (f *overlayFile) Chown(uid int, gid int) error {
	return f.writeErr("chown")
}
