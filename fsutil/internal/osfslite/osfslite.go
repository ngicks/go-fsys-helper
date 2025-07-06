package osfslite

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// OsfsLite is a lightweight OS filesystem wrapper that operates relative to a base directory.
// It provides direct access to OS filesystem operations with minimal overhead.
type OsfsLite struct {
	base string
}

// New creates a new OsfsLite filesystem rooted at the specified base directory.
func New(base string) *OsfsLite {
	return &OsfsLite{base: base}
}

func (fsys OsfsLite) Chmod(name string, mode fs.FileMode) error {
	return os.Chmod(filepath.Join(fsys.base, name), mode)
}

func (fsys OsfsLite) Chown(name string, uid int, gid int) error {
	return os.Chown(filepath.Join(fsys.base, name), uid, gid)
}

func (fsys OsfsLite) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return os.Chtimes(filepath.Join(fsys.base, name), atime, mtime)
}

func (fsys OsfsLite) Lchown(name string, uid int, gid int) error {
	return os.Lchown(filepath.Join(fsys.base, name), uid, gid)
}

func (fsys OsfsLite) Link(oldname string, newname string) error {
	return os.Link(filepath.Join(fsys.base, oldname), filepath.Join(fsys.base, newname))
}

func (fsys OsfsLite) Lstat(name string) (fs.FileInfo, error) {
	return os.Lstat(filepath.Join(fsys.base, name))
}

func (fsys OsfsLite) Mkdir(name string, perm fs.FileMode) error {
	return os.Mkdir(filepath.Join(fsys.base, name), perm)
}

func (fsys OsfsLite) MkdirAll(name string, perm fs.FileMode) error {
	return os.MkdirAll(filepath.Join(fsys.base, name), perm)
}

// Open returns an *os.File directly for filesystem operations that need the concrete type.
func (fsys OsfsLite) Open(name string) (*os.File, error) {
	return os.Open(filepath.Join(fsys.base, name))
}

func (fsys OsfsLite) Create(name string) (*os.File, error) {
	return os.Create(filepath.Join(fsys.base, name))
}

func (fsys OsfsLite) OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	return os.OpenFile(filepath.Join(fsys.base, name), flag, perm)
}

func (fsys OsfsLite) ReadLink(name string) (string, error) {
	return os.Readlink(filepath.Join(fsys.base, name))
}

func (fsys OsfsLite) Remove(name string) error {
	return os.Remove(filepath.Join(fsys.base, name))
}

func (fsys OsfsLite) RemoveAll(name string) error {
	return os.RemoveAll(filepath.Join(fsys.base, name))
}

func (fsys OsfsLite) Rename(oldname string, newname string) error {
	return os.Rename(filepath.Join(fsys.base, oldname), filepath.Join(fsys.base, newname))
}

func (fsys OsfsLite) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(filepath.Join(fsys.base, name))
}

func (fsys OsfsLite) Symlink(oldname string, newname string) error {
	return os.Symlink(oldname, filepath.Join(fsys.base, newname))
}

// FsWrapper implements fs.FS interface by wrapping OsfsLite.
// It provides fs.FS compatibility by returning fs.File instead of *os.File.
// It preserves all methods from OsfsLite that are compatible with fs interfaces.
type FsWrapper struct {
	*OsfsLite
}

// NewFsWrapper creates a new fs.FS-compatible wrapper around OsfsLite.
func NewFsWrapper(base string) *FsWrapper {
	return &FsWrapper{OsfsLite: New(base)}
}

// Open implements fs.FS interface by returning fs.File instead of *os.File.
func (w *FsWrapper) Open(name string) (fs.File, error) {
	return w.OsfsLite.Open(name)
}

// BasicWrapper implements fs.FS interface with only Open and Stat methods.
// It intentionally does NOT implement ReadLinkFs or other extended interfaces
// to simulate basic filesystems that only support Open and Stat operations.
type BasicWrapper struct {
	osfsLite *OsfsLite
}

// NewBasicWrapper creates a new basic fs.FS-compatible wrapper around OsfsLite.
func NewBasicWrapper(base string) *BasicWrapper {
	return &BasicWrapper{osfsLite: New(base)}
}

// Open implements fs.FS interface by returning fs.File instead of *os.File.
func (w *BasicWrapper) Open(name string) (fs.File, error) {
	return w.osfsLite.Open(name)
}

// Stat implements fs.StatFS interface.
func (w *BasicWrapper) Stat(name string) (fs.FileInfo, error) {
	return w.osfsLite.Stat(name)
}

