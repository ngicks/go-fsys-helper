package vroot

import (
	"errors"
	"io"
	"io/fs"
	"time"
)

var (
	// ErrPathEscapes is returned from [Fs] implementations
	// if given path escapes from the root.
	ErrPathEscapes = errors.New("path escapes from parent")
	// ErrOpNotSupported is returned from [Fs] implementations
	// if some method is not supported by the implementation.
	// The implementation still functional even without
	// specialized methods like ReadAt/WriteAt.
	// Basic operations like Fs.Open, File.Read must still be supported to be a legit
	// implementor.
	ErrOpNotSupported = errors.New("op not supported")
)

// Fs represents capablities [*os.Root] has as an interface.
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

// Unrooted is like [Rooted] but allow escaping root by sysmlink.
// Path traversals are still not allowed.
type Unrooted interface {
	Fs
	Unrooted()
	OpenUnrooted(name string) (Unrooted, error)
}

// Rooted indicates the implementation is rooted,
// which means escaping root by path traversal or symlink
// is not allowed.
type Rooted interface {
	Fs
	Rooted()
}

// File is basically same as [*os.File]
// but some system dependent methods are removed.
type File interface {
	// Chdir() error

	Chmod(mode fs.FileMode) error
	Chown(uid int, gid int) error
	Close() error

	// Fd() uintptr

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

	// SyscallConn() (syscall.RawConn, error)

	Truncate(size int64) error
	Write(b []byte) (n int, err error)
	WriteAt(b []byte, off int64) (n int, err error)
	WriteString(s string) (n int, err error)
	WriteTo(w io.Writer) (n int64, err error)
}
