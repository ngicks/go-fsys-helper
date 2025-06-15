package tarfs

import (
	"io/fs"
	"path/filepath"

	"github.com/ngicks/go-fsys-helper/fsutil"
)

func (fsys *Fs) readLinkNoResolution(name string) (string, error) {
	dirent, err := fsys.open(name)
	if err != nil {
		return "", err
	}
	return dirent.readLink()
}

func (fsys *Fs) lstatNoResolution(name string) (fs.FileInfo, error) {
	dirent, err := fsys.open(name)
	if err != nil {
		return nil, err
	}
	return dirent.header().Header().FileInfo(), nil
}

func (fsys *Fs) resolvePath(name string, skipLastElement bool) (string, error) {
	return fsutil.ResolvePath(&filepathTarFs{fsys}, name, skipLastElement)
}

type filepathTarFs struct {
	fsys *Fs
}

func (fsys *filepathTarFs) ReadLink(name string) (string, error) {
	return fsys.fsys.readLinkNoResolution(filepath.ToSlash(name))
}

func (fsys *filepathTarFs) Lstat(name string) (fs.FileInfo, error) {
	return fsys.fsys.lstatNoResolution(filepath.ToSlash(name))
}
