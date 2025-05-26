package vroot

import (
	"errors"
	"io"
	"io/fs"
	"time"
)

var ErrPathEscapes = errors.New("path escapes from parent")

type Fs interface {
	Chmod(name string, mode fs.FileMode) error
	Chown(name string, uid int, gid int) error
	Chtimes(name string, atime time.Time, mtime time.Time) error
	Close() error
	Create(name string) (File, error)
	Lchown(name string, uid int, gid int) error
	Link(oldname string, newname string) error
	Lstat(name string) (fs.FileInfo, error)
	Mkdir(name string, perm fs.FileMode) error
	MkdirAll(name string, perm fs.FileMode) error
	Name() string
	Open(name string) (File, error)
	OpenFile(name string, flag int, perm fs.FileMode) (File, error)
	OpenRoot(name string) (Rooted, error)
	Readlink(name string) (string, error)
	Remove(name string) error
	RemoveAll(name string) error
	Rename(oldname string, newname string) error
	Stat(name string) (fs.FileInfo, error)
	Symlink(oldname string, newname string) error
}

type Unrooted interface {
	Fs
	Unrooted()
	OpenUnrooted(name string) (Unrooted, error)
}

type Rooted interface {
	Fs
	Rooted()
}

type File interface {
	// Chdir() error

	Chmod(mode fs.FileMode) error
	Chown(uid int, gid int) error
	Close() error

	//	Fd() uintptr

	Name() string
	Read(b []byte) (n int, err error)
	ReadAt(b []byte, off int64) (n int, err error)
	ReadDir(n int) ([]fs.DirEntry, error)
	ReadFrom(r io.Reader) (n int64, err error)
	Readdir(n int) ([]fs.FileInfo, error)
	Readdirnames(n int) (names []string, err error)
	Seek(offset int64, whence int) (ret int64, err error)

	// SetDeadline(t time.Time) error
	// SetReadDeadline(t time.Time) error
	// SetWriteDeadline(t time.Time) error

	Stat() (fs.FileInfo, error)
	Sync() error

	//	SyscallConn() (syscall.RawConn, error)

	Truncate(size int64) error
	Write(b []byte) (n int, err error)
	WriteAt(b []byte, off int64) (n int, err error)
	WriteString(s string) (n int, err error)
	WriteTo(w io.Writer) (n int64, err error)
}
