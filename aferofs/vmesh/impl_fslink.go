package vmesh

import (
	"io/fs"
	"os"
	"syscall"

	"github.com/ngicks/go-fsys-helper/aferofs"
	"github.com/spf13/afero"
)

var _ FileData = (*fsLinkFileData)(nil)

type fsLinkFileData struct {
	fsys fs.FS
	path string
}

func (b *fsLinkFileData) Close() error {
	return nil
}

// NewFsLinkFileData builds FileData that points a file stored in fsys referred as path.
func NewFsLinkFileData(fsys fs.FS, path string) (FileData, error) {
	s, err := fs.Stat(fsys, path)
	if err != nil {
		return nil, err
	}
	if s.IsDir() {
		return nil, &fs.PathError{Op: "NewFsLinkFileData", Path: path, Err: syscall.EISDIR}
	}
	if !s.Mode().IsRegular() {
		return nil, &fs.PathError{Op: "NewFsLinkFileData", Path: path, Err: syscall.EBADF}
	}
	return &fsLinkFileData{fsys, path}, nil
}

func (b *fsLinkFileData) Open(flag int) (afero.File, error) {
	if flag&(os.O_WRONLY|syscall.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, syscall.EROFS
	}
	f, err := b.fsys.Open(b.path)
	if err != nil {
		return nil, err
	}
	return aferofs.NewFsFile(f, b.path, true), nil
}

func (b *fsLinkFileData) Stat() (fs.FileInfo, error) {
	return fs.Stat(b.fsys, b.path)
}

func (b *fsLinkFileData) Truncate(size int64) error {
	return syscall.EROFS
}
