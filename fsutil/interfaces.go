// Package fsutil defines filesystem abstraction library agnostic helpers
package fsutil

import (
	"io/fs"
	"time"
)

// Fs files

type ChmodFs interface {
	Chmod(name string, mode fs.FileMode) error
}

type ChownFs interface {
	Chown(name string, uid int, gid int) error
}

type ChtimesFs interface {
	Chtimes(name string, atime time.Time, mtime time.Time) error
}

type LchownFs interface {
	Lchown(name string, uid int, gid int) error
}

type LinkFs interface {
	Link(oldname string, newname string) error
}

type LstatFs interface {
	Lstat(name string) (fs.FileInfo, error)
}

type MkdirFs interface {
	Mkdir(name string, perm fs.FileMode) error
}

type MkdirAllFs interface {
	MkdirAll(name string, perm fs.FileMode) error
}

type OpenFileFs[File any] interface {
	OpenFile(name string, flag int, perm fs.FileMode) (File, error)
}

type ReadLinkFs interface {
	ReadLink(name string) (string, error)
}

type RemoveFs interface {
	Remove(name string) error
}

type RemoveAllFs interface {
	RemoveAll(name string) error
}

type RenameFs interface {
	Rename(oldname string, newname string) error
}

type StatFs interface {
	Stat(name string) (fs.FileInfo, error)
}

type SymlinkFs interface {
	Symlink(oldname string, newname string) error
}

// File interfaces

type ChmodFile interface {
	Chmod(mode fs.FileMode) error
}

type ChownFile interface {
	Chown(uid int, gid int) error
}

type CloseFile interface {
	Close() error
}

type NameFile interface {
	Name() string
}

type ReadFile interface {
	Read(b []byte) (n int, err error)
}

type ReadAtFile interface {
	ReadAt(b []byte, off int64) (n int, err error)
}

type ReadDirFile interface {
	ReadDir(n int) ([]fs.DirEntry, error)
}

type ReaddirFile interface {
	Readdir(n int) ([]fs.FileInfo, error)
}

type ReaddirnamesFile interface {
	Readdirnames(n int) (names []string, err error)
}

type SeekFile interface {
	Seek(offset int64, whence int) (ret int64, err error)
}

type StatFile interface {
	Stat() (fs.FileInfo, error)
}

type SyncFile interface {
	Sync() error
}

type TruncateFile interface {
	Truncate(size int64) error
}

type WriteFile interface {
	Write(b []byte) (n int, err error)
}

type WriteAtFile interface {
	WriteAt(b []byte, off int64) (n int, err error)
}

type WriteStringFile interface {
	WriteString(s string) (n int, err error)
}
