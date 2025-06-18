package synthfs

import (
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
)

var _ direntry = (*symlink)(nil)

type symlink struct {
	metadata
	target string
}

func (s *symlink) open(flag int) (openDirentry, error) {
	// Symlinks should not be opened directly - they should be resolved first
	return nil, fsutil.WrapPathErr("open", s.s.name, syscall.ELOOP)
}

func (s *symlink) readLink() (string, error) {
	return s.target, nil
}
