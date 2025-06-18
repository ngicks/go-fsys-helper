package synthfs

import (
	"io/fs"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// direntry is static, stateless entry in the [*fsys].
type direntry interface {
	chmod(mode fs.FileMode)
	chown(uid, gid int)
	chtimes(atime time.Time, mtime time.Time) error
	rename(newname string)
	stat() (fs.FileInfo, error)
	owner() (uid, gid int)
	open(flag int) (openDirentry, error)
	readLink() (string, error)
}

// openDirentry is the stateful file opened through [direntry].
// It has states, i.e. offset for file reading or ReadDir.
type openDirentry interface {
	vroot.File
}
