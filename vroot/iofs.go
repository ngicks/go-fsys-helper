package vroot

import (
	"io/fs"
	"path/filepath"
)

var (
	_ fs.FS         = (*IoFsRooted)(nil)
	_ fs.ReadDirFS  = (*IoFsRooted)(nil)
	_ fs.ReadFileFS = (*IoFsRooted)(nil)
	_ fs.ReadLinkFS = (*IoFsRooted)(nil)
	_ fs.StatFS     = (*IoFsRooted)(nil)
	_ fs.SubFS      = (*IoFsRooted)(nil)
)

type IoFsRooted struct {
	root Rooted
}

func NewIoFsRooted(root Rooted) *IoFsRooted {
	return &IoFsRooted{root: root}
}

func (fsys *IoFsRooted) Close() error {
	return fsys.root.Close()
}

func (fsys *IoFsRooted) Open(name string) (fs.File, error) {
	return newReadOnlyFile(fsys.root.Open(filepath.FromSlash(name)))
}

func (fsys *IoFsRooted) ReadDir(name string) ([]fs.DirEntry, error) {
	return ReadDir(fsys.root, name)
}

func (fsys *IoFsRooted) ReadFile(name string) ([]byte, error) {
	return ReadFile(fsys.root, name)
}

func (fsys *IoFsRooted) ReadLink(name string) (string, error) {
	return fsys.root.Readlink(filepath.FromSlash(name))
}

func (fsys *IoFsRooted) Lstat(name string) (fs.FileInfo, error) {
	return fsys.root.Lstat(filepath.FromSlash(name))
}

func (fsys *IoFsRooted) Stat(name string) (fs.FileInfo, error) {
	return fsys.root.Stat(filepath.FromSlash(name))
}

func (fsys *IoFsRooted) Sub(dir string) (fs.FS, error) {
	root, err := fsys.root.OpenRoot(filepath.FromSlash(dir))
	if err != nil {
		return nil, err
	}
	return NewIoFsRooted(root), nil
}

type IoFsUnrooted struct {
	root Unrooted
}

func NewIoFsUnrooted(root Unrooted) *IoFsUnrooted {
	return &IoFsUnrooted{root: root}
}

func (fsys *IoFsUnrooted) Close() error {
	return fsys.root.Close()
}

func (fsys *IoFsUnrooted) Open(name string) (fs.File, error) {
	return newReadOnlyFile(fsys.root.Open(filepath.FromSlash(name)))
}

func (fsys *IoFsUnrooted) ReadDir(name string) ([]fs.DirEntry, error) {
	return ReadDir(fsys.root, name)
}

func (fsys *IoFsUnrooted) ReadFile(name string) ([]byte, error) {
	return ReadFile(fsys.root, name)
}

func (fsys *IoFsUnrooted) ReadLink(name string) (string, error) {
	return fsys.root.Readlink(filepath.FromSlash(name))
}

func (fsys *IoFsUnrooted) Lstat(name string) (fs.FileInfo, error) {
	return fsys.root.Lstat(filepath.FromSlash(name))
}

func (fsys *IoFsUnrooted) Stat(name string) (fs.FileInfo, error) {
	return fsys.root.Stat(filepath.FromSlash(name))
}

func (fsys *IoFsUnrooted) Sub(dir string) (fs.FS, error) {
	root, err := fsys.root.OpenUnrooted(filepath.FromSlash(dir))
	if err != nil {
		return nil, err
	}
	return NewIoFsUnrooted(root), nil
}
