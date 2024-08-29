// Code generated by github.com/ngicks/go-f-helper/aferofs/cmd/implwrapper. DO NOT EDIT.
package implwrapper

import (
	"io/fs"
)

func (recv *B2) Close() (err error) {
	if checkErr := recv.beforeEach("Close"); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.inner.Close()

	if checkErr := recv.afterEach("Close", err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Close", err)

	return
}

func (recv *B2) Name() (s string) {
	_ = recv.beforeEach("Name")

	s = recv.inner.Name()
	_ = recv.afterEach("Name", s)
	s = recv.modifyString("Name", s)

	return
}

func (recv *B2) Read(p []byte) (n int, err error) {
	if checkErr := recv.beforeEach("Read", p); checkErr != nil {
		err = checkErr
		return
	}

	p = recv.modifyP("Read", p)

	n, err = recv.inner.Read(p)

	if checkErr := recv.afterEach("Read", n, err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Read", err)
	n = recv.modifyN("Read", n)

	return
}

func (recv *B2) ReadAt(p []byte, off int64) (n int, err error) {
	if checkErr := recv.beforeEach("ReadAt", p, off); checkErr != nil {
		err = checkErr
		return
	}

	p = recv.modifyP("ReadAt", p)
	off = recv.modifyOff("ReadAt", off)

	n, err = recv.inner.ReadAt(p, off)

	if checkErr := recv.afterEach("ReadAt", n, err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("ReadAt", err)
	n = recv.modifyN("ReadAt", n)

	return
}

func (recv *B2) Readdir(count int) (fi []fs.FileInfo, err error) {
	if checkErr := recv.beforeEach("Readdir", count); checkErr != nil {
		err = checkErr
		return
	}

	fi, err = recv.inner.Readdir(count)

	if checkErr := recv.afterEach("Readdir", fi, err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Readdir", err)
	fi = recv.modifyFi("Readdir", fi)

	return
}

func (recv *B2) Readdirnames(n int) (s []string, err error) {
	if checkErr := recv.beforeEach("Readdirnames", n); checkErr != nil {
		err = checkErr
		return
	}

	n = recv.modifyN("Readdirnames", n)

	s, err = recv.inner.Readdirnames(n)

	if checkErr := recv.afterEach("Readdirnames", s, err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Readdirnames", err)
	s = recv.modifyDirnames("Readdirnames", s)

	return
}

func (recv *B2) Seek(offset int64, whence int) (n int64, err error) {
	if checkErr := recv.beforeEach("Seek", offset, whence); checkErr != nil {
		err = checkErr
		return
	}

	n, err = recv.inner.Seek(offset, whence)

	if checkErr := recv.afterEach("Seek", n, err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Seek", err)

	return
}

func (recv *B2) Stat() (fi fs.FileInfo, err error) {
	if checkErr := recv.beforeEach("Stat"); checkErr != nil {
		err = checkErr
		return
	}

	fi, err = recv.inner.Stat()

	if checkErr := recv.afterEach("Stat", fi, err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Stat", err)
	fi = recv.modifyFi("Stat", []fs.FileInfo{fi})[0]

	return
}

func (recv *B2) Sync() (err error) {
	if checkErr := recv.beforeEach("Sync"); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.inner.Sync()

	if checkErr := recv.afterEach("Sync", err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Sync", err)

	return
}

func (recv *B2) Truncate(size int64) (err error) {
	if checkErr := recv.beforeEach("Truncate", size); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.inner.Truncate(size)

	if checkErr := recv.afterEach("Truncate", err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Truncate", err)

	return
}

func (recv *B2) Write(p []byte) (n int, err error) {
	if checkErr := recv.beforeEach("Write", p); checkErr != nil {
		err = checkErr
		return
	}

	p = recv.modifyP("Write", p)

	n, err = recv.inner.Write(p)

	if checkErr := recv.afterEach("Write", n, err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("Write", err)
	n = recv.modifyN("Write", n)

	return
}

func (recv *B2) WriteAt(p []byte, off int64) (n int, err error) {
	if checkErr := recv.beforeEach("WriteAt", p, off); checkErr != nil {
		err = checkErr
		return
	}

	p = recv.modifyP("WriteAt", p)
	off = recv.modifyOff("WriteAt", off)

	n, err = recv.inner.WriteAt(p, off)

	if checkErr := recv.afterEach("WriteAt", n, err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("WriteAt", err)
	n = recv.modifyN("WriteAt", n)

	return
}

func (recv *B2) WriteString(s string) (n int, err error) {
	if checkErr := recv.beforeEach("WriteString", s); checkErr != nil {
		err = checkErr
		return
	}

	s = recv.modifyString("WriteString", s)

	n, err = recv.inner.WriteString(s)

	if checkErr := recv.afterEach("WriteString", n, err); checkErr != nil {
		err = checkErr
		return
	}

	err = recv.modifyErr("WriteString", err)
	n = recv.modifyN("WriteString", n)

	return
}
