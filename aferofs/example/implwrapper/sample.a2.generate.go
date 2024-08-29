// Code generated by github.com/ngicks/go-f-helper/aferofs/cmd/implwrapper. DO NOT EDIT.
package implwrapper

import (
	"io/fs"
	"time"

	"github.com/spf13/afero"
)

func (recv *A2) Create(name string) (f afero.File, err error) {
	if checkErr := recv.beforeEach("Create", name); checkErr != nil {
		err = checkErr
		return
	}

	name, _ = recv.modifyPath("Create", name, "")

	f, err = recv.inner.Create(name)

	if checkErr := recv.afterEach("Create", f, err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Create", err)
	f = recv.modifyFile("Create", f)

	return
}

func (recv *A2) Mkdir(name string, perm fs.FileMode) (err error) {
	if checkErr := recv.beforeEach("Mkdir", name, perm); checkErr != nil {
		err = checkErr
		return
	}

	name, _ = recv.modifyPath("Mkdir", name, "")
	perm = recv.modifyMode("Mkdir", perm)

	err = recv.inner.Mkdir(name, perm)

	if checkErr := recv.afterEach("Mkdir", err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Mkdir", err)

	return
}

func (recv *A2) MkdirAll(path string, perm fs.FileMode) (err error) {
	if checkErr := recv.beforeEach("MkdirAll", path, perm); checkErr != nil {
		err = checkErr
		return
	}

	path, _ = recv.modifyPath("MkdirAll", path, "")
	perm = recv.modifyMode("MkdirAll", perm)

	err = recv.inner.MkdirAll(path, perm)

	if checkErr := recv.afterEach("MkdirAll", err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("MkdirAll", err)

	return
}

func (recv *A2) Open(name string) (f afero.File, err error) {
	if checkErr := recv.beforeEach("Open", name); checkErr != nil {
		err = checkErr
		return
	}

	name, _ = recv.modifyPath("Open", name, "")

	f, err = recv.inner.Open(name)

	if checkErr := recv.afterEach("Open", f, err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Open", err)
	f = recv.modifyFile("Open", f)

	return
}

func (recv *A2) OpenFile(name string, flag int, perm fs.FileMode) (f afero.File, err error) {
	if checkErr := recv.beforeEach("OpenFile", name, flag, perm); checkErr != nil {
		err = checkErr
		return
	}

	name, _ = recv.modifyPath("OpenFile", name, "")
	perm = recv.modifyMode("OpenFile", perm)

	f, err = recv.inner.OpenFile(name, flag, perm)

	if checkErr := recv.afterEach("OpenFile", f, err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("OpenFile", err)
	f = recv.modifyFile("OpenFile", f)

	return
}

func (recv *A2) Remove(name string) (err error) {
	if checkErr := recv.beforeEach("Remove", name); checkErr != nil {
		err = checkErr
		return
	}

	name, _ = recv.modifyPath("Remove", name, "")

	err = recv.inner.Remove(name)

	if checkErr := recv.afterEach("Remove", err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Remove", err)

	return
}

func (recv *A2) RemoveAll(path string) (err error) {
	if checkErr := recv.beforeEach("RemoveAll", path); checkErr != nil {
		err = checkErr
		return
	}

	path, _ = recv.modifyPath("RemoveAll", path, "")

	err = recv.inner.RemoveAll(path)

	if checkErr := recv.afterEach("RemoveAll", err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("RemoveAll", err)

	return
}

func (recv *A2) Rename(oldname string, newname string) (err error) {
	if checkErr := recv.beforeEach("Rename", oldname, newname); checkErr != nil {
		err = checkErr
		return
	}

	oldname, newname = recv.modifyPath("Rename", oldname, newname)

	err = recv.inner.Rename(oldname, newname)

	if checkErr := recv.afterEach("Rename", err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Rename", err)

	return
}

func (recv *A2) Stat(name string) (fi fs.FileInfo, err error) {
	if checkErr := recv.beforeEach("Stat", name); checkErr != nil {
		err = checkErr
		return
	}

	name, _ = recv.modifyPath("Stat", name, "")

	fi, err = recv.inner.Stat(name)

	if checkErr := recv.afterEach("Stat", fi, err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Stat", err)
	fi = recv.modifyFi("Stat", []fs.FileInfo{fi})[0]

	return
}

func (recv *A2) Name() (name string) {
	_ = recv.beforeEach("Name")

	name = recv.inner.Name()
	_ = recv.afterEach("Name", name)
	name, _ = recv.modifyPath("Name", name, "")

	return
}

func (recv *A2) Chmod(name string, mode fs.FileMode) (err error) {
	if checkErr := recv.beforeEach("Chmod", name, mode); checkErr != nil {
		err = checkErr
		return
	}

	name, _ = recv.modifyPath("Chmod", name, "")
	mode = recv.modifyMode("Chmod", mode)

	err = recv.inner.Chmod(name, mode)

	if checkErr := recv.afterEach("Chmod", err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Chmod", err)

	return
}

func (recv *A2) Chown(name string, uid int, gid int) (err error) {
	if checkErr := recv.beforeEach("Chown", name, uid, gid); checkErr != nil {
		err = checkErr
		return
	}

	name, _ = recv.modifyPath("Chown", name, "")

	err = recv.inner.Chown(name, uid, gid)

	if checkErr := recv.afterEach("Chown", err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Chown", err)

	return
}

func (recv *A2) Chtimes(name string, atime time.Time, mtime time.Time) (err error) {
	if checkErr := recv.beforeEach("Chtimes", name, atime, mtime); checkErr != nil {
		err = checkErr
		return
	}

	name, _ = recv.modifyPath("Chtimes", name, "")
	atime, mtime = recv.modifyTimes("Chtimes", atime, mtime)

	err = recv.inner.Chtimes(name, atime, mtime)

	if checkErr := recv.afterEach("Chtimes", err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Chtimes", err)

	return
}
