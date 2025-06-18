package vroot

import (
	"io/fs"
	"path/filepath"
)

var (
	_ fs.FS         = (*ioFsToRooted)(nil)
	_ fs.ReadDirFS  = (*ioFsToRooted)(nil)
	_ fs.ReadFileFS = (*ioFsToRooted)(nil)
	_ fs.ReadLinkFS = (*ioFsToRooted)(nil)
	_ fs.StatFS     = (*ioFsToRooted)(nil)
	_ fs.SubFS      = (*ioFsToRooted)(nil)
)

type ioFsToRooted struct {
	root Rooted
}

// ToIoFsRooted narrows [Fs] so that it can be used as [fs.FS].
//
// With all write methods removed, returned [fs.FS] and [fs.File] can not be type-asserted
// to writable interface.
//
// The returned [fs.FS] implements [fs.SubFS].
// The method returns rooted sub file system.
func ToIoFsRooted(root Rooted) fs.FS {
	return &ioFsToRooted{root: root}
}

func (fsys *ioFsToRooted) Close() error {
	return fsys.root.Close()
}

func (fsys *ioFsToRooted) Open(name string) (fs.File, error) {
	return NewReadOnlyFile(fsys.root.Open(filepath.FromSlash(name)))
}

func (fsys *ioFsToRooted) ReadDir(name string) ([]fs.DirEntry, error) {
	return ReadDir(fsys.root, name)
}

func (fsys *ioFsToRooted) ReadFile(name string) ([]byte, error) {
	return ReadFile(fsys.root, name)
}

func (fsys *ioFsToRooted) ReadLink(name string) (string, error) {
	return fsys.root.ReadLink(filepath.FromSlash(name))
}

func (fsys *ioFsToRooted) Lstat(name string) (fs.FileInfo, error) {
	return fsys.root.Lstat(filepath.FromSlash(name))
}

func (fsys *ioFsToRooted) Stat(name string) (fs.FileInfo, error) {
	return fsys.root.Stat(filepath.FromSlash(name))
}

func (fsys *ioFsToRooted) Sub(dir string) (fs.FS, error) {
	root, err := fsys.root.OpenRoot(filepath.FromSlash(dir))
	if err != nil {
		return nil, err
	}
	return ToIoFsRooted(root), nil
}

type ioFsToUnrooted struct {
	root Unrooted
}

// ToIoFsUnrooted narrows [Fs] so that it can be used as [fs.FS].
//
// With all write methods removed, returned [fs.FS] and [fs.File] can not be type-asserted
// to writable interface.
//
// The returned [fs.FS] implements [fs.SubFS].
// The method returns unrooted sub file system.
func ToIoFsUnrooted(root Unrooted) fs.FS {
	return &ioFsToUnrooted{root: root}
}

func (fsys *ioFsToUnrooted) Close() error {
	return fsys.root.Close()
}

func (fsys *ioFsToUnrooted) Open(name string) (fs.File, error) {
	return NewReadOnlyFile(fsys.root.Open(filepath.FromSlash(name)))
}

func (fsys *ioFsToUnrooted) ReadDir(name string) ([]fs.DirEntry, error) {
	return ReadDir(fsys.root, name)
}

func (fsys *ioFsToUnrooted) ReadFile(name string) ([]byte, error) {
	return ReadFile(fsys.root, name)
}

func (fsys *ioFsToUnrooted) ReadLink(name string) (string, error) {
	return fsys.root.ReadLink(filepath.FromSlash(name))
}

func (fsys *ioFsToUnrooted) Lstat(name string) (fs.FileInfo, error) {
	return fsys.root.Lstat(filepath.FromSlash(name))
}

func (fsys *ioFsToUnrooted) Stat(name string) (fs.FileInfo, error) {
	return fsys.root.Stat(filepath.FromSlash(name))
}

func (fsys *ioFsToUnrooted) Sub(dir string) (fs.FS, error) {
	root, err := fsys.root.OpenUnrooted(filepath.FromSlash(dir))
	if err != nil {
		return nil, err
	}
	return ToIoFsUnrooted(root), nil
}
