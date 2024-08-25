package vmesh

import (
	"github.com/ngicks/go-fsys-helper/aferofs/internal/closable"
	"github.com/spf13/afero"
)

func newFd(t afero.File) *closable.Closable[afero.File] {
	return closable.NewFile[afero.File](t)
}

func newOpenHandle(path string, flag int, d *dirent) (*closable.Closable[afero.File], error) {
	if d.dir != nil {
		return newFd(&dirHandle{
			dir:  d.dir,
			name: path,
		}), nil
	} else {
		f, err := d.file.Open(flag)
		if err != nil {
			return nil, err
		}
		return newFd(f), nil
	}
}
