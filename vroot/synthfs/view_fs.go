package synthfs

import (
	"fmt"
	"io"
	"io/fs"
	"path"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
)

// NewFsView returns a read-only [FileView] over the regular file at name
// inside fsys. The view captures fsys and name; if the underlying file
// changes, subsequent Open calls see the new content.
//
// Returns an error if name is not a regular file in fsys.
func NewFsView(fsys fs.FS, name string) (FileView, error) {
	v, err := newFsView(fsys, name)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func newFsView(fsys fs.FS, name string) (*fsView, error) {
	info, err := fs.Stat(fsys, name)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, &fs.PathError{Op: "NewFsView", Path: name, Err: syscall.EISDIR}
	}
	if !info.Mode().IsRegular() {
		return nil, &fs.PathError{Op: "NewFsView", Path: name, Err: errdef.EBADF}
	}
	return &fsView{fsys: fsys, name: name}, nil
}

type fsView struct {
	fsys fs.FS
	name string
}

func (v *fsView) Open(flag int) (vroot.File, error) {
	if openflag.WriteOp(flag) {
		return nil, errdef.EROFS
	}
	f, err := v.fsys.Open(v.name)
	if err != nil {
		return nil, err
	}
	return vroot.ExpandFsFile(f, v.name), nil
}

func (v *fsView) Stat() (fs.FileInfo, error) {
	return fs.Stat(v.fsys, v.name)
}

func (v *fsView) Close() error {
	return nil
}

// NewRangedView returns a read-only [FileView] exposing bytes [off, off+n) of
// inner. The inner view must report an [io.ReaderAt]-capable handle (the
// default in-memory and bytes views both do). off must be non-negative; n
// must be positive.
func NewRangedView(inner FileView, off, n int64) (FileView, error) {
	if off < 0 {
		return nil, fmt.Errorf("synthfs: NewRangedView: off must be non-negative, got %d", off)
	}
	if n <= 0 {
		return nil, fmt.Errorf("synthfs: NewRangedView: n must be positive, got %d", n)
	}
	// Probe: confirm inner.Open returns a ReaderAt.
	h, err := inner.Open(0)
	if err != nil {
		return nil, fmt.Errorf("synthfs: NewRangedView probe: %w", err)
	}
	defer func() { _ = h.Close() }()
	if _, ok := any(h).(io.ReaderAt); !ok {
		return nil, fmt.Errorf("synthfs: NewRangedView: inner view does not support ReadAt")
	}
	return &rangedView{inner: inner, off: off, n: n}, nil
}

type rangedView struct {
	inner FileView
	off   int64
	n     int64
}

func (v *rangedView) Open(flag int) (vroot.File, error) {
	if openflag.WriteOp(flag) {
		return nil, errdef.EROFS
	}
	inner, err := v.inner.Open(flag)
	if err != nil {
		return nil, err
	}
	ra, ok := any(inner).(io.ReaderAt)
	if !ok {
		_ = inner.Close()
		return nil, fmt.Errorf("synthfs: rangedView: inner handle does not support ReadAt")
	}
	innerStat, err := inner.Stat()
	if err != nil {
		_ = inner.Close()
		return nil, err
	}
	sr := io.NewSectionReader(ra, v.off, v.n)
	return &rangedHandle{
		inner: inner,
		sr:    sr,
		name:  innerStat.Name(),
	}, nil
}

func (v *rangedView) Stat() (fs.FileInfo, error) {
	info, err := v.inner.Stat()
	if err != nil {
		return nil, err
	}
	return stat{
		name:    path.Base(info.Name()),
		size:    v.n,
		mode:    info.Mode(),
		modTime: info.ModTime(),
	}, nil
}

func (v *rangedView) Close() error {
	return v.inner.Close()
}

type rangedHandle struct {
	inner vroot.File
	sr    *io.SectionReader
	name  string
}

var _ vroot.File = (*rangedHandle)(nil)

func (h *rangedHandle) Name() string {
	return h.name
}

func (h *rangedHandle) Fd() uintptr               { return ^uintptr(0) }
func (h *rangedHandle) Chmod(fs.FileMode) error   { return errdef.EROFS }
func (h *rangedHandle) Chown(int, int) error      { return errdef.EROFS }
func (h *rangedHandle) Truncate(int64) error      { return errdef.EROFS }
func (h *rangedHandle) Write([]byte) (int, error) { return 0, errdef.EROFS }
func (h *rangedHandle) WriteAt([]byte, int64) (int, error) {
	return 0, errdef.EROFS
}
func (h *rangedHandle) WriteString(string) (int, error) { return 0, errdef.EROFS }
func (h *rangedHandle) Sync() error                     { return nil }

func (h *rangedHandle) Close() error {
	return h.inner.Close()
}

func (h *rangedHandle) Read(p []byte) (int, error) {
	return h.sr.Read(p)
}

func (h *rangedHandle) ReadAt(p []byte, off int64) (int, error) {
	return h.sr.ReadAt(p, off)
}

func (h *rangedHandle) Seek(offset int64, whence int) (int64, error) {
	return h.sr.Seek(offset, whence)
}

func (h *rangedHandle) Stat() (fs.FileInfo, error) {
	info, err := h.inner.Stat()
	if err != nil {
		return nil, err
	}
	return stat{
		name:    path.Base(info.Name()),
		size:    h.sr.Size(),
		mode:    info.Mode(),
		modTime: info.ModTime(),
	}, nil
}

func (h *rangedHandle) ReadDir(int) ([]fs.DirEntry, error) {
	return nil, fsutil.WrapPathErr("readdir", h.name, syscall.ENOTDIR)
}

func (h *rangedHandle) Readdir(int) ([]fs.FileInfo, error) {
	return nil, fsutil.WrapPathErr("readdir", h.name, syscall.ENOTDIR)
}

func (h *rangedHandle) Readdirnames(int) ([]string, error) {
	return nil, fsutil.WrapPathErr("readdir", h.name, syscall.ENOTDIR)
}
