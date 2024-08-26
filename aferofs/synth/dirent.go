package synth

import (
	"io/fs"
	pathPkg "path"
	"syscall"
	"time"
)

// A readonly struct. no locks.
type dirent struct {
	// base name of dirent.
	name string
	// non-nil if is a dir.
	dir *dir
	// non-nil if is a file.
	file *virtualFileData
}

func newDirDirent(name string, mode fs.FileMode, modTime time.Time, dirents ...*dirent) *dirent {
	return &dirent{
		name: name,
		dir:  newDirData(mode, modTime, dirents...),
	}
}

func newFileDirent(data FileView, path string) (*dirent, error) {
	vf, err := newVirtualFileData(data, pathPkg.Base(path))
	if err != nil {
		return nil, err
	}
	return &dirent{name: pathPkg.Base(path), file: vf}, nil
}

func (d *dirent) IsSearchableDir() error {
	if err := d.DoesExist(); err != nil {
		return err
	}
	if err := d.IsDirErr(); err != nil {
		return err
	}
	if !d.hasPerm(0o1) {
		return syscall.EACCES
	}
	return nil
}

func (d *dirent) IsWritableDir() error {
	err := d.IsSearchableDir()
	if err != nil {
		return err
	}
	if !d.hasPerm(0o2) {
		return syscall.EACCES
	}
	return nil
}

func (d *dirent) DoesExist() error {
	if d == nil || (d.dir == nil && d.file == nil) {
		return syscall.ENOENT
	}
	return nil
}

func (d *dirent) IsValid() error {
	if err := d.DoesExist(); err != nil {
		return syscall.EINVAL
	}
	return nil
}

func (d *dirent) IsDirErr() error {
	if d.dir == nil {
		return syscall.ENOTDIR
	}
	return nil
}

func (d *dirent) IsFileErr() error {
	if d.file == nil {
		return syscall.EBADF
	}
	return nil
}

func (d *dirent) IsDir() bool {
	return d.dir != nil
}

func (d *dirent) IsFile() bool {
	return !d.IsDir()
}

func (d *dirent) IsReadable() error {
	if !d.hasPerm(0o4) {
		return syscall.EPERM
	}
	return nil
}

func (d *dirent) hasPerm(userPerm int) bool {
	var perm fs.FileMode
	if d.dir != nil {
		perm = d.dir.Mode()
	}
	if d.file != nil {
		perm = d.file.Mode()
	}
	targetPerm := fs.FileMode(userPerm & 0o7)
	return perm.Perm()>>6&targetPerm == targetPerm
}

func (d *dirent) lookup(name string) (ent *dirent, ok bool) {
	return d.dir.lookup(name)
}

func (d *dirent) stat() (fs.FileInfo, error) {
	if d.dir != nil {
		return d.dir.Stat(d.name)
	} else {
		return d.file.Stat()
	}
}

func (d *dirent) chmod(mode fs.FileMode) {
	if d.dir != nil {
		d.dir.Chmod(mode)
	}
	if d.file != nil {
		d.file.Chmod(mode)
	}
}

func (d *dirent) chown(uid, gid int) {
	if d.dir != nil {
		d.dir.Chown(uid, gid)
	}
	if d.file != nil {
		d.file.Chown(uid, gid)
	}
}

func (d *dirent) chtimes(atime time.Time, mtime time.Time) {
	if d.dir != nil {
		d.dir.Chtimes(atime, mtime)
	}
	if d.file != nil {
		d.file.Chtimes(atime, mtime)
	}
}

func (d *dirent) copyMeta(u *dirent) {
	d.chmod(u.mode())
	d.chown(u.owner())
	d.chtimes(u.times())
}

func (d *dirent) addDirent(u *dirent) (replaced *dirent) {
	return d.dir.AddDirent(u)
}

func (d *dirent) removeDirent(u *dirent) {
	d.dir.RemoveDirent(u)
}

func (d *dirent) removeName(name string) {
	d.dir.RemoveName(name)
}

func (d *dirent) notifyRename(newname string) {
	if d.IsFile() {
		d.file.notifyRename(newname)
	}
}

func (d *dirent) len() int {
	if d.IsDir() {
		return d.dir.Len()
	}
	return 0
}

func (d *dirent) notifyClose() error {
	if d.IsFile() {
		return d.file.notifyClose()
	} else {
		d.dir.notifyClose()
		return nil
	}
}

func (d *dirent) mode() fs.FileMode {
	if d.IsFile() {
		return d.file.Mode()
	} else {
		return d.dir.Mode()
	}
}

func (d *dirent) owner() (uid, gid int) {
	if d.IsFile() {
		return d.file.Owner()
	} else {
		return d.dir.Owner()
	}
}

func (d *dirent) times() (atime, mtime time.Time) {
	if d.IsFile() {
		return d.file.Times()
	} else {
		return d.dir.Times()
	}
}
