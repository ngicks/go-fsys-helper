package aferofs

import (
	"io/fs"
	"os"
	"syscall"
	"time"

	"github.com/spf13/afero"
)

var (
	_ fs.FS         = (*IoFs)(nil)
	_ fs.ReadFileFS = (*IoFs)(nil)
	_ fs.StatFS     = (*IoFs)(nil)
	_ fs.SubFS      = (*IoFs)(nil)
)

type IoFs struct {
	afero.Fs
}

func (i *IoFs) Open(name string) (fs.File, error) {
	if err := ValidPathErr(name); err != nil {
		return nil, err
	}

	f, err := i.Fs.Open(name)
	if err != nil {
		return nil, err
	}
	return &readDirFile{f}, nil
}

var _ fs.ReadDirFile = (*readDirFile)(nil)

type readDirFile struct {
	afero.File
}

func (r *readDirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	fi, err := r.File.Readdir(n)
	if err != nil {
		return []fs.DirEntry{}, err
	}
	dirents := make([]fs.DirEntry, len(fi))
	for i, fi := range fi {
		dirents[i] = &dirent{fi}
	}
	return dirents, nil
}

var _ fs.DirEntry = (*dirent)(nil)

type dirent struct {
	fs.FileInfo
}

func (d *dirent) Info() (fs.FileInfo, error) {
	return d, nil
}

func (d *dirent) Type() fs.FileMode {
	return d.Mode().Type()
}

func (i *IoFs) ReadFile(name string) ([]byte, error) {
	if err := ValidPathErr(name); err != nil {
		return nil, err
	}
	return afero.ReadFile(i.Fs, name)
}

func (i *IoFs) Sub(dir string) (fs.FS, error) {
	if err := ValidPathErr(dir); err != nil {
		return nil, err
	}
	return &IoFs{Fs: afero.NewBasePathFs(i.Fs, dir)}, nil
}

var _ afero.Fs = (*IoFsAdapter)(nil)

type IoFsAdapter struct {
	fs.FS
}

func NewIoFsAdapter(fsys fs.FS, readonly bool) afero.Fs {
	return &IoFsAdapter{fsys}
}

func readonlyFsysErr(op, name string) error {
	return &fs.PathError{Op: op, Path: name, Err: syscall.EROFS}
}

func (i *IoFsAdapter) Chmod(name string, mode fs.FileMode) error {
	return readonlyFsysErr("chmod", name)
}

func (i *IoFsAdapter) Chown(name string, uid int, gid int) error {
	return readonlyFsysErr("chown", name)
}

func (i *IoFsAdapter) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return readonlyFsysErr("chtimes", name)
}

func (i *IoFsAdapter) Create(name string) (afero.File, error) {
	return nil, readonlyFsysErr("create", name)
}

func (i *IoFsAdapter) Mkdir(name string, perm fs.FileMode) error {
	return readonlyFsysErr("mkdir", name)
}

func (i *IoFsAdapter) MkdirAll(path string, perm fs.FileMode) error {
	return readonlyFsysErr("mkdir", path)
}

func (i *IoFsAdapter) Name() string {
	return "IoFsAdapter"
}

func (i *IoFsAdapter) Open(name string) (afero.File, error) {
	if err := ValidPathErr(name); err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	f, err := i.FS.Open(name)
	if err != nil {
		return nil, err
	}
	return NewFsFile(f, name, true), nil
}

func (i *IoFsAdapter) OpenFile(name string, flag int, perm fs.FileMode) (afero.File, error) {
	if flag&(os.O_WRONLY|syscall.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, &fs.PathError{Op: "open", Path: name, Err: syscall.EROFS}
	}
	return i.Open(name)
}

func (i *IoFsAdapter) Remove(name string) error {
	return readonlyFsysErr("remove", name)
}

func (i *IoFsAdapter) RemoveAll(path string) error {
	return readonlyFsysErr("remove", path)
}

func (i *IoFsAdapter) Rename(oldname string, newname string) error {
	return readonlyFsysErr("rename", oldname)
}

func (i *IoFsAdapter) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(i.FS, name)
}

var _ afero.File = (*FsFile)(nil)

type FsFile struct {
	fs.File
	path     string
	readonly bool
}

func NewFsFile(f fs.File, path string, readonly bool) afero.File {
	return &FsFile{f, path, readonly}
}

func (f *FsFile) Name() string {
	return f.path
}

func (f *FsFile) ReadAt(p []byte, off int64) (n int, err error) {
	return ReadAt(f.File, p, off)
}

func (f *FsFile) Readdir(count int) ([]fs.FileInfo, error) {
	return Readdir(f.File, count)
}

func (f *FsFile) Readdirnames(n int) ([]string, error) {
	return Readdirnames(f.File, n)
}

func (f *FsFile) Seek(offset int64, whence int) (int64, error) {
	return Seek(f.File, offset, whence)
}

func (f *FsFile) Sync() error {
	return Sync(f.File)
}

func (f *FsFile) Truncate(size int64) error {
	if f.readonly {
		return readonlyFsysErr("truncate", f.path)
	}
	return Truncate(f.File, size)
}

func (f *FsFile) Write(p []byte) (n int, err error) {
	if f.readonly {
		return 0, readonlyFsysErr("write", f.path)
	}
	return Write(f.File, p)
}

func (f *FsFile) WriteAt(p []byte, off int64) (n int, err error) {
	if f.readonly {
		return 0, readonlyFsysErr("writeat", f.path)
	}
	return WriteAt(f.File, p, off)
}

func (f *FsFile) WriteString(s string) (ret int, err error) {
	if f.readonly {
		return 0, readonlyFsysErr("write", f.path)
	}
	return WriteString(f.File, s)
}
