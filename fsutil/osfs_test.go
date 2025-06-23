package fsutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type osfsLite struct {
	base string
}

func (fsys osfsLite) Chmod(name string, mode fs.FileMode) error {
	return os.Chmod(filepath.Join(fsys.base, name), mode)
}

func (fsys osfsLite) Chown(name string, uid int, gid int) error {
	return os.Chown(filepath.Join(fsys.base, name), uid, gid)
}

func (fsys osfsLite) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return os.Chtimes(filepath.Join(fsys.base, name), atime, mtime)
}

func (fsys osfsLite) Lchown(name string, uid int, gid int) error {
	return os.Lchown(filepath.Join(fsys.base, name), uid, gid)
}

func (fsys osfsLite) Link(oldname string, newname string) error {
	return os.Link(filepath.Join(fsys.base, oldname), filepath.Join(fsys.base, newname))
}

func (fsys osfsLite) Lstat(name string) (fs.FileInfo, error) {
	return os.Lstat(filepath.Join(fsys.base, name))
}

func (fsys osfsLite) Mkdir(name string, perm fs.FileMode) error {
	return os.Mkdir(filepath.Join(fsys.base, name), perm)
}

func (fsys osfsLite) MkdirAll(name string, perm fs.FileMode) error {
	return os.MkdirAll(filepath.Join(fsys.base, name), perm)
}

func (fsys osfsLite) OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	return os.OpenFile(filepath.Join(fsys.base, name), flag, perm)
}

func (fsys osfsLite) ReadLink(name string) (string, error) {
	return os.Readlink(filepath.Join(fsys.base, name))
}

func (fsys osfsLite) Remove(name string) error {
	return os.Remove(filepath.Join(fsys.base, name))
}

func (fsys osfsLite) RemoveAll(name string) error {
	return os.RemoveAll(filepath.Join(fsys.base, name))
}

func (fsys osfsLite) Rename(oldname string, newname string) error {
	return os.Rename(filepath.Join(fsys.base, oldname), filepath.Join(fsys.base, newname))
}

func (fsys osfsLite) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(filepath.Join(fsys.base, name))
}

func (fsys osfsLite) Symlink(oldname string, newname string) error {
	return os.Symlink(oldname, filepath.Join(fsys.base, newname))
}
