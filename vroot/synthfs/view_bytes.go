package synthfs

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
)

// NewBytesView returns a read-only [FileView] over a byte slice. The slice is
// not copied; callers must not mutate it after binding the view. mode and mt
// are reported through Stat — callers usually pass 0o444 and the current time.
func NewBytesView(b []byte, mode fs.FileMode, mt time.Time) FileView {
	return &bytesView{data: b, mode: mode & fs.ModePerm, mt: mt}
}

type bytesView struct {
	data []byte
	mode fs.FileMode
	mt   time.Time
}

func (v *bytesView) Open(flag int) (vroot.File, error) {
	if openflag.WriteOp(flag) {
		return nil, errdef.EROFS
	}
	return &bytesHandle{view: v, r: bytes.NewReader(v.data)}, nil
}

func (v *bytesView) Stat() (fs.FileInfo, error) {
	return stat{size: int64(len(v.data)), mode: v.mode, modTime: v.mt}, nil
}

func (v *bytesView) Close() error {
	return nil
}

type bytesHandle struct {
	view *bytesView
	r    *bytes.Reader
}

var _ vroot.File = (*bytesHandle)(nil)

func (h *bytesHandle) Name() string {
	// bytesView is not tied to a path until it's bound into the tree; the
	// tree-side wrapper supplies the user-facing name. Return "" here so
	// nothing silently uses an incorrect path.
	return ""
}

func (h *bytesHandle) Fd() uintptr               { return ^uintptr(0) }
func (h *bytesHandle) Close() error              { return nil }
func (h *bytesHandle) Sync() error               { return nil }
func (h *bytesHandle) Chmod(fs.FileMode) error   { return errdef.EROFS }
func (h *bytesHandle) Chown(int, int) error      { return errdef.EROFS }
func (h *bytesHandle) Truncate(int64) error      { return errdef.EROFS }
func (h *bytesHandle) Write([]byte) (int, error) { return 0, errdef.EROFS }
func (h *bytesHandle) WriteAt([]byte, int64) (int, error) {
	return 0, errdef.EROFS
}
func (h *bytesHandle) WriteString(string) (int, error) { return 0, errdef.EROFS }

func (h *bytesHandle) Read(p []byte) (int, error) {
	return h.r.Read(p)
}

func (h *bytesHandle) ReadAt(p []byte, off int64) (int, error) {
	return h.r.ReadAt(p, off)
}

func (h *bytesHandle) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart, io.SeekCurrent, io.SeekEnd:
		off, err := h.r.Seek(offset, whence)
		if err != nil {
			return off, fsutil.WrapPathErr("seek", h.Name(), err)
		}
		return off, nil
	default:
		return 0, fsutil.WrapPathErr(
			"seek",
			h.Name(),
			fmt.Errorf("unknown whence %d: %w", whence, fs.ErrInvalid),
		)
	}
}

func (h *bytesHandle) Stat() (fs.FileInfo, error) {
	return stat{size: h.r.Size(), mode: h.view.mode, modTime: h.view.mt}, nil
}

func (h *bytesHandle) ReadDir(int) ([]fs.DirEntry, error) {
	return nil, fsutil.WrapPathErr("readdir", h.Name(), syscall.ENOTDIR)
}

func (h *bytesHandle) Readdir(int) ([]fs.FileInfo, error) {
	return nil, fsutil.WrapPathErr("readdir", h.Name(), syscall.ENOTDIR)
}

func (h *bytesHandle) Readdirnames(int) ([]string, error) {
	return nil, fsutil.WrapPathErr("readdir", h.Name(), syscall.ENOTDIR)
}
