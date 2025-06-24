package overlayfs

import (
	"errors"
	"io/fs"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
)

var ErrWhitedOut = errors.New("whited out")

var _ vroot.Rooted = (*Layer)(nil)

// Layer is read-only [vroot.Rooted] but
// treats files/directories as non existent if it is listed in whilte out file list stored in [MetadataStore].
type Layer struct {
	meta MetadataStore
	fsys vroot.Rooted
}

// NewLayer creates a new Layer with the given metadata store and filesystem
func NewLayer(meta MetadataStore, fsys vroot.Rooted) Layer {
	return Layer{
		meta: meta,
		fsys: fsys,
	}
}

// isWhitedOut checks if a path is whited out in this layer
func (l *Layer) isWhitedOut(name string) (bool, error) {
	return l.meta.QueryWhiteout(name)
}

// Rooted marks this as a rooted filesystem
func (l *Layer) Rooted() {}

// Name returns the name of the underlying filesystem
func (l *Layer) Name() string {
	return l.fsys.Name()
}

// Close closes the underlying filesystem
func (l *Layer) Close() error {
	return l.fsys.Close()
}

// Stat returns file info, respecting whiteouts
func (l *Layer) Stat(name string) (fs.FileInfo, error) {
	whited, err := l.isWhitedOut(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("stat", name, err)
	}
	if whited {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: ErrWhitedOut}
	}
	return l.fsys.Stat(name)
}

// Lstat returns file info without following symlinks, respecting whiteouts
func (l *Layer) Lstat(name string) (fs.FileInfo, error) {
	whited, err := l.isWhitedOut(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("lstat", name, err)
	}
	if whited {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: ErrWhitedOut}
	}
	return l.fsys.Lstat(name)
}

// Open opens a file for reading, respecting whiteouts
func (l *Layer) Open(name string) (vroot.File, error) {
	whited, err := l.isWhitedOut(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	if whited {
		return nil, &fs.PathError{Op: "open", Path: name, Err: ErrWhitedOut}
	}
	return l.fsys.Open(name)
}

// OpenFile opens a file with flags, respecting whiteouts
func (l *Layer) OpenFile(name string, flag int, perm fs.FileMode) (vroot.File, error) {
	// Check for write flags - return EROFS for any write operations
	if openflag.WriteOp(flag) {
		return nil, fsutil.WrapPathErr("open", name, errdef.EROFS)
	}

	whited, err := l.isWhitedOut(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	if whited {
		return nil, &fs.PathError{Op: "open", Path: name, Err: ErrWhitedOut}
	}
	return l.fsys.OpenFile(name, flag, perm)
}

// Create creates a new file (read-only - returns EROFS)
func (l *Layer) Create(name string) (vroot.File, error) {
	return nil, fsutil.WrapPathErr("open", name, errdef.EROFS)
}

// Remove removes a file (read-only - returns EROFS)
func (l *Layer) Remove(name string) error {
	return fsutil.WrapPathErr("remove", name, errdef.EROFS)
}

// RemoveAll removes a directory tree (read-only - returns EROFS)
func (l *Layer) RemoveAll(name string) error {
	return fsutil.WrapPathErr("removeall", name, errdef.EROFS)
}

// Mkdir creates a directory (read-only - returns EROFS)
func (l *Layer) Mkdir(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, errdef.EROFS)
}

// MkdirAll creates a directory tree (read-only - returns EROFS)
func (l *Layer) MkdirAll(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, errdef.EROFS)
}

// Rename renames a file (read-only - returns EROFS)
func (l *Layer) Rename(oldname, newname string) error {
	return fsutil.WrapLinkErr("rename", oldname, newname, errdef.EROFS)
}

// Link creates a hard link (read-only - returns EROFS)
func (l *Layer) Link(oldname, newname string) error {
	return fsutil.WrapLinkErr("link", oldname, newname, errdef.EROFS)
}

// Symlink creates a symbolic link (read-only - returns EROFS)
func (l *Layer) Symlink(oldname, newname string) error {
	return fsutil.WrapLinkErr("symlink", oldname, newname, errdef.EROFS)
}

// ReadLink reads a symbolic link, respecting whiteouts
func (l *Layer) ReadLink(name string) (string, error) {
	whited, err := l.isWhitedOut(name)
	if err != nil {
		return "", fsutil.WrapPathErr("readlink", name, err)
	}
	if whited {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: ErrWhitedOut}
	}
	return l.fsys.ReadLink(name)
}

// Chmod changes file permissions (read-only - returns EROFS)
func (l *Layer) Chmod(name string, mode fs.FileMode) error {
	return fsutil.WrapPathErr("chmod", name, errdef.EROFS)
}

// Chown changes file ownership (read-only - returns EROFS)
func (l *Layer) Chown(name string, uid, gid int) error {
	return fsutil.WrapPathErr("chown", name, errdef.EROFS)
}

// Lchown changes file ownership without following symlinks (read-only - returns EROFS)
func (l *Layer) Lchown(name string, uid, gid int) error {
	return fsutil.WrapPathErr("lchown", name, errdef.EROFS)
}

// Chtimes changes file access and modification times (read-only - returns EROFS)
func (l *Layer) Chtimes(name string, atime, mtime time.Time) error {
	return fsutil.WrapPathErr("chtimes", name, errdef.EROFS)
}

// OpenRoot opens a subdirectory as a new rooted filesystem, respecting whiteouts
func (l *Layer) OpenRoot(name string) (vroot.Rooted, error) {
	whited, err := l.isWhitedOut(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("openroot", name, err)
	}
	if whited {
		return nil, &fs.PathError{Op: "openroot", Path: name, Err: ErrWhitedOut}
	}

	subRoot, err := l.fsys.OpenRoot(name)
	if err != nil {
		return nil, err
	}

	// Return a new Layer wrapping the sub-root
	return &Layer{
		meta: l.meta,
		fsys: subRoot,
	}, nil
}
