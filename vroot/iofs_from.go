package vroot

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
)

var (
	_ Rooted   = (*ioFsFromRooted)(nil)
	_ Unrooted = (*ioFsFromUnrooted)(nil)
)

type ioFsFromRooted struct {
	fsys fs.ReadLinkFS
	name string
}

// FromIoFsRooted widens [fs.FS] so that it can be used as [Rooted].
// name is directly retunred from Name method.
//
// The returned [Rooted] provides read-only access and
// write methods, e.g. Chmod, Chtimes and Write on [File],
// return an error that satisfies, for methods on [Fs], errors.Is(err, syscall.EROFS)
// and for methods on [File], errors.Is(err, syscall.EPERM).
//
// Although it is "Rooted", it is still vulnerable to TOCTOU(Time-of-Check-Time-of-Use) race:
// symlinks are evaluated by sequence of Lstat and ReadLink calls.
// The target could be changed between check and actual evaluation by functions like Open, Stat, etc.
func FromIoFsRooted(fsys fs.ReadLinkFS, name string) Rooted {
	return &ioFsFromRooted{
		fsys: fsys,
		name: name,
	}
}

func (f *ioFsFromRooted) Rooted() {}

func (f *ioFsFromRooted) resolvePath(name string, skipLastElement bool) (string, error) {
	return fsutil.ResolvePath(f.fsys, name, skipLastElement)
}

func (f *ioFsFromRooted) Chmod(name string, mode fs.FileMode) error {
	return fsutil.WrapPathErr("chmod", name, syscall.EROFS)
}

func (f *ioFsFromRooted) Chown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("chown", name, syscall.EROFS)
}

func (f *ioFsFromRooted) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return fsutil.WrapPathErr("chtimes", name, syscall.EROFS)
}

func (f *ioFsFromRooted) Close() error {
	return nil
}

func (f *ioFsFromRooted) Create(name string) (File, error) {
	return nil, fsutil.WrapPathErr("open", name, syscall.EROFS)
}

func (f *ioFsFromRooted) Lchown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("lchown", name, syscall.EROFS)
}

func (f *ioFsFromRooted) Link(oldname string, newname string) error {
	return fsutil.WrapLinkErr("link", oldname, newname, syscall.EROFS)
}

func (f *ioFsFromRooted) Lstat(name string) (fs.FileInfo, error) {
	path, err := f.resolvePath(name, true)
	if err != nil {
		return nil, fsutil.WrapPathErr("lstat", name, err)
	}
	return f.fsys.Lstat(path)
}

func (f *ioFsFromRooted) Mkdir(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, syscall.EROFS)
}

func (f *ioFsFromRooted) MkdirAll(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, syscall.EROFS)
}

func (f *ioFsFromRooted) Name() string {
	return f.name
}

func (f *ioFsFromRooted) Open(name string) (File, error) {
	path, err := f.resolvePath(name, false)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	file, err := f.fsys.Open(path)
	if err != nil {
		return nil, err
	}
	return NewFsFile(file, path), nil
}

func (f *ioFsFromRooted) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	if flag&(os.O_WRONLY|syscall.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, fsutil.WrapPathErr("open", name, syscall.EROFS)
	}
	return f.Open(name)
}

func (f *ioFsFromRooted) OpenRoot(name string) (Rooted, error) {
	subPath, err := f.resolvePath(name, false)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}

	subFsys, err := fs.Sub(f.fsys, subPath)
	if err != nil {
		return nil, err
	}

	readLinkFsys, ok := subFsys.(fs.ReadLinkFS)
	if !ok {
		return nil, fmt.Errorf("*ioFsFromRooted.OpenRoot: sub fsys does not implement fs.ReadLinkFS")
	}

	return FromIoFsRooted(readLinkFsys, path.Join(f.name, name)), nil
}

func (f *ioFsFromRooted) ReadLink(name string) (string, error) {
	resolved, err := f.resolvePath(name, true)
	if err != nil {
		return "", fsutil.WrapPathErr("readlink", name, err)
	}
	return f.fsys.ReadLink(resolved)
}

func (f *ioFsFromRooted) Remove(name string) error {
	return fsutil.WrapPathErr("remove", name, syscall.EROFS)
}

func (f *ioFsFromRooted) RemoveAll(name string) error {
	return fsutil.WrapPathErr("RemoveAll", name, syscall.EROFS)
}

func (f *ioFsFromRooted) Rename(oldname string, newname string) error {
	return fsutil.WrapLinkErr("rename", oldname, newname, syscall.EROFS)
}

func (f *ioFsFromRooted) Stat(name string) (fs.FileInfo, error) {
	path, err := f.resolvePath(name, false)
	if err != nil {
		return nil, fsutil.WrapPathErr("stat", name, err)
	}
	return fs.Stat(f.fsys, path)
}

func (f *ioFsFromRooted) Symlink(oldname string, newname string) error {
	return fsutil.WrapLinkErr("symlink", oldname, newname, syscall.EROFS)
}

type ioFsFromUnrooted struct {
	fsys fs.ReadLinkFS
	name string
}

// FromIoFsUnrooted widens [fs.FS] so that it can be used as [Unrooted].
// name is directly retunred from Name method.
//
// The returned [Unrooted] provides read-only access and
// write methods, e.g. Chmod, Chtimes and Write on [File],
// return an error that satisfies, for methods on [Fs], errors.Is(err, syscall.EROFS)
// and for methods on [File], errors.Is(err, syscall.EPERM).
func FromIoFsUnrooted(fsys fs.ReadLinkFS, name string) Unrooted {
	return &ioFsFromUnrooted{
		fsys: fsys,
		name: name,
	}
}

func (f *ioFsFromUnrooted) Unrooted() {}

func (f *ioFsFromUnrooted) resolvePath(name string) (string, error) {
	name = filepath.Clean(name)

	if !filepath.IsLocal(name) {
		return "", ErrPathEscapes
	}

	return filepath.ToSlash(name), nil
}

func (f *ioFsFromUnrooted) Chmod(name string, mode fs.FileMode) error {
	return fsutil.WrapPathErr("chmod", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Chown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("chown", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return fsutil.WrapPathErr("chtimes", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Close() error {
	return nil
}

func (f *ioFsFromUnrooted) Create(name string) (File, error) {
	return nil, fsutil.WrapPathErr("open", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Lchown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("lchown", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Link(oldname string, newname string) error {
	return fsutil.WrapLinkErr("link", oldname, newname, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Lstat(name string) (fs.FileInfo, error) {
	path, err := f.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("lstat", name, err)
	}
	return f.fsys.Lstat(path)
}

func (f *ioFsFromUnrooted) Mkdir(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) MkdirAll(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Name() string {
	return f.name
}

func (f *ioFsFromUnrooted) Open(name string) (File, error) {
	path, err := f.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	file, err := f.fsys.Open(path)
	if err != nil {
		return nil, err
	}
	return NewFsFile(file, path), nil
}

func (f *ioFsFromUnrooted) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	if flag&(os.O_WRONLY|syscall.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, fsutil.WrapPathErr("open", name, syscall.EROFS)
	}
	return f.Open(name)
}

func (f *ioFsFromUnrooted) OpenRoot(name string) (Rooted, error) {
	subPath, err := f.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}

	subFsys, err := fs.Sub(f.fsys, subPath)
	if err != nil {
		return nil, err
	}

	readLinkFsys, ok := subFsys.(fs.ReadLinkFS)
	if !ok {
		return nil, fmt.Errorf("*ioFsFromUnrooted.OpenRoot: sub fsys does not implement fs.ReadLinkFS")
	}

	return FromIoFsRooted(readLinkFsys, path.Join(f.name, name)), nil
}

func (f *ioFsFromUnrooted) ReadLink(name string) (string, error) {
	resolved, err := f.resolvePath(name)
	if err != nil {
		return "", fsutil.WrapPathErr("readlink", name, err)
	}
	return f.fsys.ReadLink(resolved)
}

func (f *ioFsFromUnrooted) Remove(name string) error {
	return fsutil.WrapPathErr("remove", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) RemoveAll(name string) error {
	return fsutil.WrapPathErr("RemoveAll", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Rename(oldname string, newname string) error {
	return fsutil.WrapLinkErr("rename", oldname, newname, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Stat(name string) (fs.FileInfo, error) {
	path, err := f.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("stat", name, err)
	}
	return fs.Stat(f.fsys, path)
}

func (f *ioFsFromUnrooted) Symlink(oldname string, newname string) error {
	return fsutil.WrapLinkErr("symlink", oldname, newname, syscall.EROFS)
}

func (f *ioFsFromUnrooted) OpenUnrooted(name string) (Unrooted, error) {
	subPath, err := f.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}

	subFsys, err := fs.Sub(f.fsys, subPath)
	if err != nil {
		return nil, err
	}

	readLinkFsys, ok := subFsys.(fs.ReadLinkFS)
	if !ok {
		return nil, fmt.Errorf("*ioFsFromUnrooted.OpenUnrooted: sub fsys does not implement fs.ReadLinkFS")
	}

	return FromIoFsUnrooted(readLinkFsys, path.Join(f.name, name)), nil
}

var _ File = (*FsFile)(nil)

type FsFile struct {
	file fs.File
	name string
}

func NewFsFile(file fs.File, name string) *FsFile {
	return &FsFile{file: file, name: name}
}

func (f *FsFile) pathErr(op string) error {
	return fsutil.WrapPathErr(op, f.name, syscall.EPERM)
}

func (f *FsFile) Chmod(mode fs.FileMode) error {
	return f.pathErr("chmod")
}

func (f *FsFile) Chown(uid int, gid int) error {
	return f.pathErr("chown")
}

func (f *FsFile) Close() error {
	return f.file.Close()
}

func (f *FsFile) Name() string {
	return f.name
}

func (f *FsFile) Fd() uintptr {
	return Fd(f.file)
}

func (f *FsFile) Read(b []byte) (n int, err error) {
	return f.file.Read(b)
}

func (f *FsFile) ReadAt(b []byte, off int64) (n int, err error) {
	if ra, ok := f.file.(io.ReaderAt); ok {
		return ra.ReadAt(b, off)
	}
	return 0, fsutil.WrapPathErr("readat", f.name, ErrOpNotSupported)
}

func (f *FsFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if readDirFile, ok := f.file.(fs.ReadDirFile); ok {
		return readDirFile.ReadDir(n)
	}
	return nil, fsutil.WrapPathErr("readdir", f.name, errors.New("not implemented"))
}

func (f *FsFile) Readdir(n int) ([]fs.FileInfo, error) {
	entries, err := f.ReadDir(n)
	if err != nil {
		return nil, err
	}

	infos := make([]fs.FileInfo, len(entries))
	for i, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		infos[i] = info
	}
	return infos, nil
}

func (f *FsFile) Readdirnames(n int) (names []string, err error) {
	entries, err := f.ReadDir(n)
	if err != nil {
		return nil, err
	}

	names = make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names, nil
}

func (f *FsFile) Seek(offset int64, whence int) (ret int64, err error) {
	if s, ok := f.file.(io.Seeker); ok {
		return s.Seek(offset, whence)
	}
	return 0, fsutil.WrapPathErr("seek", f.name, ErrOpNotSupported)
}

func (f *FsFile) Stat() (fs.FileInfo, error) {
	return f.file.Stat()
}

func (f *FsFile) Sync() error {
	return f.pathErr("sync")
}

func (f *FsFile) Truncate(size int64) error {
	return f.pathErr("truncate")
}

func (f *FsFile) Write(b []byte) (n int, err error) {
	return 0, f.pathErr("write")
}

func (f *FsFile) WriteAt(b []byte, off int64) (n int, err error) {
	return 0, f.pathErr("write")
}

func (f *FsFile) WriteString(s string) (n int, err error) {
	return 0, f.pathErr("write")
}
