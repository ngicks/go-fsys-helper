package synthfs

import (
	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
)

var _ direntry = (*symlink)(nil)

type symlink struct {
	metadata
	target string
}

func (s *symlink) open(flag int) (openDirentry, error) {
	return nil, fsutil.WrapPathErr("open", s.s.name, errdef.EBADF)
}

func (s *symlink) readLink() (string, error) {
	return s.target, nil
}
