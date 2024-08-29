// Code generated by github.com/ngicks/go-fsys-helper/aferofs/cmd/implwrapper. DO NOT EDIT.
package aferofs

import (
	"os"
	"time"

	"github.com/spf13/afero"
)

func (fsys *ValidPathFs) Create(name string) (f afero.File, err error) {
	name, _ = fsys.modifyPath("Create", name, "")
	f, err = fsys.inner.Create(name)
	return
}

func (fsys *ValidPathFs) Mkdir(name string, perm os.FileMode) (err error) {
	name, _ = fsys.modifyPath("Mkdir", name, "")
	err = fsys.inner.Mkdir(name, perm)
	return
}

func (fsys *ValidPathFs) MkdirAll(path string, perm os.FileMode) (err error) {
	path, _ = fsys.modifyPath("MkdirAll", path, "")
	err = fsys.inner.MkdirAll(path, perm)
	return
}

func (fsys *ValidPathFs) Open(name string) (f afero.File, err error) {
	name, _ = fsys.modifyPath("Open", name, "")
	f, err = fsys.inner.Open(name)
	return
}

func (fsys *ValidPathFs) OpenFile(name string, flag int, perm os.FileMode) (f afero.File, err error) {
	name, _ = fsys.modifyPath("OpenFile", name, "")
	f, err = fsys.inner.OpenFile(name, flag, perm)
	return
}

func (fsys *ValidPathFs) Remove(name string) (err error) {
	name, _ = fsys.modifyPath("Remove", name, "")
	err = fsys.inner.Remove(name)
	return
}

func (fsys *ValidPathFs) RemoveAll(path string) (err error) {
	path, _ = fsys.modifyPath("RemoveAll", path, "")
	err = fsys.inner.RemoveAll(path)
	return
}

func (fsys *ValidPathFs) Rename(oldname, newname string) (err error) {
	oldname, newname = fsys.modifyPath("Rename", oldname, newname)
	err = fsys.inner.Rename(oldname, newname)
	return
}

func (fsys *ValidPathFs) Stat(name string) (fi os.FileInfo, err error) {
	name, _ = fsys.modifyPath("Stat", name, "")
	fi, err = fsys.inner.Stat(name)
	return
}

func (fsys *ValidPathFs) Name() (s string) {
	s = fsys.inner.Name()
	s, _ = fsys.modifyPath("Name", s, "")
	return
}

func (fsys *ValidPathFs) Chmod(name string, mode os.FileMode) (err error) {
	name, _ = fsys.modifyPath("Chmod", name, "")
	err = fsys.inner.Chmod(name, mode)
	return
}

func (fsys *ValidPathFs) Chown(name string, uid, gid int) (err error) {
	name, _ = fsys.modifyPath("Chown", name, "")
	err = fsys.inner.Chown(name, uid, gid)
	return
}

func (fsys *ValidPathFs) Chtimes(name string, atime time.Time, mtime time.Time) (err error) {
	name, _ = fsys.modifyPath("Chtimes", name, "")
	err = fsys.inner.Chtimes(name, atime, mtime)
	return
}
