package vroot

import (
	_ "io"
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
	return narrowFile(fsys.root.Open(filepath.FromSlash(name)))
}

func (fsys *ioFsToRooted) ReadDir(name string) ([]fs.DirEntry, error) {
	return ReadDir(fsys.root, filepath.FromSlash(name))
}

func (fsys *ioFsToRooted) ReadFile(name string) ([]byte, error) {
	return ReadFile(fsys.root, filepath.FromSlash(name))
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
	return ReadDir(fsys.root, filepath.FromSlash(name))
}

func (fsys *ioFsToUnrooted) ReadFile(name string) ([]byte, error) {
	return ReadFile(fsys.root, filepath.FromSlash(name))
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

type fsFile struct {
	f File
}

type fsFileReaderAt struct {
	*fsFile
}

// NarrowFile narrows [File] capability to [fs.File]
//
// If calling ReadAt on f return [ErrOpNotSupported],
// returned fs.File does not implemnet [io.ReaderAt] and [io.Seeker].
func NarrowFile(f File) fs.File {
	var b [1]byte
	_, readAtErr := f.ReadAt(b[:], 0)
	if readAtErr == nil { // may return ErrOpNotSupported
		// assumption: io.ReaderAt implementor also implements Seeker.
		// You can easily implement it by wrapping file with [io.SectionReader]
		return &fsFileReaderAt{&fsFile{f: f}}
	}
	return &fsFile{f: f}
}

func narrowFile(f File, err error) (fs.File, error) {
	if f == nil {
		return nil, err
	}
	return NarrowFile(f), nil
}

func (r *fsFile) Close() error {
	return r.f.Close()
}

func (r *fsFile) Name() string {
	return r.f.Name()
}

func (r *fsFile) Read(b []byte) (n int, err error) {
	return r.f.Read(b)
}

func (r *fsFile) ReadDir(n int) ([]fs.DirEntry, error) {
	return r.f.ReadDir(n)
}

func (r *fsFile) Readdir(n int) ([]fs.FileInfo, error) {
	return r.f.Readdir(n)
}

func (r *fsFile) Readdirnames(n int) (names []string, err error) {
	return r.f.Readdirnames(n)
}

func (r *fsFile) Stat() (fs.FileInfo, error) {
	return r.f.Stat()
}

func (r *fsFileReaderAt) Seek(offset int64, whence int) (ret int64, err error) {
	return r.f.Seek(offset, whence)
}

func (r *fsFileReaderAt) ReadAt(b []byte, off int64) (n int, err error) {
	return r.f.ReadAt(b, off)
}
