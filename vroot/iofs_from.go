package vroot

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot/internal/wrapper"
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
// return error that satisfies, for methods on [Fs], errors.Is(err, syscall.EROFS)
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
	name = filepath.Clean(name)

	if name == "." {
		return ".", nil
	}

	if !filepath.IsLocal(name) {
		return "", ErrPathEscapes
	}

	parts := strings.Split(name, string(filepath.Separator))
	currentPath := ""

	var lastPart string
	if skipLastElement { // for readlink, lstat, lchown
		lastPart = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	}

	for _, part := range parts {
		if currentPath == "" {
			currentPath = part
		} else {
			currentPath = currentPath + string(filepath.Separator) + part
		}

		info, err := f.fsys.Lstat(currentPath)
		if err != nil {
			return "", err
		}

		if info.Mode()&fs.ModeSymlink == 0 {
			continue
		}

		resolved, err := resolveSymlink(f.fsys, currentPath)
		if err != nil {
			return "", err
		}

		if resolved == "" || !filepath.IsLocal(resolved) {
			// Target is absolute or has "..".
			// *os.Root rejects this anyway, since it cannot tell final result is within root.
			// *os.Root depends on at variants of syscalls, e.g. openat.
			// The root directory may be moved after open,
			// but you don't have robust way to convert an fd back to a path on the filesystem.
			return "", ErrPathEscapes
		}

		currentPath = resolved
	}

	if lastPart != "" {
		if currentPath != "" {
			currentPath += string(filepath.Separator)
		}
		currentPath += lastPart
	}

	return filepath.ToSlash(currentPath), nil
}

func (f *ioFsFromRooted) Chmod(name string, mode fs.FileMode) error {
	return wrapper.PathErr("chmod", name, syscall.EROFS)
}

func (f *ioFsFromRooted) Chown(name string, uid int, gid int) error {
	return wrapper.PathErr("chown", name, syscall.EROFS)
}

func (f *ioFsFromRooted) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return wrapper.PathErr("chtimes", name, syscall.EROFS)
}

func (f *ioFsFromRooted) Close() error {
	return nil
}

func (f *ioFsFromRooted) Create(name string) (File, error) {
	return nil, wrapper.PathErr("open", name, syscall.EROFS)
}

func (f *ioFsFromRooted) Lchown(name string, uid int, gid int) error {
	return wrapper.PathErr("lchown", name, syscall.EROFS)
}

func (f *ioFsFromRooted) Link(oldname string, newname string) error {
	return wrapper.LinkErr("link", oldname, newname, syscall.EROFS)
}

func (f *ioFsFromRooted) Lstat(name string) (fs.FileInfo, error) {
	path, err := f.resolvePath(name, true)
	if err != nil {
		return nil, wrapper.PathErr("lstat", name, err)
	}
	return f.fsys.Lstat(path)
}

func (f *ioFsFromRooted) Mkdir(name string, perm fs.FileMode) error {
	return wrapper.PathErr("mkdir", name, syscall.EROFS)
}

func (f *ioFsFromRooted) MkdirAll(name string, perm fs.FileMode) error {
	return wrapper.PathErr("mkdir", name, syscall.EROFS)
}

func (f *ioFsFromRooted) Name() string {
	return f.name
}

func (f *ioFsFromRooted) Open(name string) (File, error) {
	path, err := f.resolvePath(name, false)
	if err != nil {
		return nil, wrapper.PathErr("open", name, err)
	}
	file, err := f.fsys.Open(path)
	if err != nil {
		return nil, err
	}
	return newFsFile(file, path), nil
}

func (f *ioFsFromRooted) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	if flag&(os.O_WRONLY|syscall.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, wrapper.PathErr("open", name, syscall.EROFS)
	}
	return f.Open(name)
}

func (f *ioFsFromRooted) OpenRoot(name string) (Rooted, error) {
	subPath, err := f.resolvePath(name, false)
	if err != nil {
		return nil, wrapper.PathErr("open", name, err)
	}

	subFsys, err := fs.Sub(f.fsys, subPath)
	if err != nil {
		return nil, err
	}

	readLinkFsys, ok := subFsys.(fs.ReadLinkFS)
	if !ok {
		return nil, fmt.Errorf("*FsRooted.OpenRoot: sub fsys does not implement fs.ReadLinkFS")
	}

	return FromIoFsRooted(readLinkFsys, path.Join(f.name, name)), nil
}

func (f *ioFsFromRooted) ReadLink(name string) (string, error) {
	resolved, err := f.resolvePath(name, true)
	if err != nil {
		return "", wrapper.PathErr("readlink", name, err)
	}
	return f.fsys.ReadLink(resolved)
}

func (f *ioFsFromRooted) Remove(name string) error {
	return wrapper.PathErr("remove", name, syscall.EROFS)
}

func (f *ioFsFromRooted) RemoveAll(name string) error {
	return wrapper.PathErr("RemoveAll", name, syscall.EROFS)
}

func (f *ioFsFromRooted) Rename(oldname string, newname string) error {
	return wrapper.LinkErr("rename", oldname, newname, syscall.EROFS)
}

func (f *ioFsFromRooted) Stat(name string) (fs.FileInfo, error) {
	path, err := f.resolvePath(name, false)
	if err != nil {
		return nil, wrapper.PathErr("stat", name, err)
	}
	return fs.Stat(f.fsys, path)
}

func (f *ioFsFromRooted) Symlink(oldname string, newname string) error {
	return wrapper.LinkErr("symlink", oldname, newname, syscall.EROFS)
}

// fsFile wraps an fs.File to implement vroot.File
type fsFile struct {
	file fs.File
	name string
}

var _ File = (*fsFile)(nil)

func newFsFile(file fs.File, name string) *fsFile {
	return &fsFile{file: file, name: name}
}

func (f *fsFile) pathErr(op string) error {
	return wrapper.PathErr(op, f.name, syscall.EPERM)
}

func (f *fsFile) Chmod(mode fs.FileMode) error {
	return f.pathErr("chmod")
}

func (f *fsFile) Chown(uid int, gid int) error {
	return f.pathErr("chown")
}

func (f *fsFile) Close() error {
	return f.file.Close()
}

func (f *fsFile) Name() string {
	return f.name
}

func (f *fsFile) Read(b []byte) (n int, err error) {
	return f.file.Read(b)
}

func (f *fsFile) ReadAt(b []byte, off int64) (n int, err error) {
	if ra, ok := f.file.(io.ReaderAt); ok {
		return ra.ReadAt(b, off)
	}
	return 0, wrapper.PathErr("readat", f.name, ErrOpNotSupported)
}

func (f *fsFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if readDirFile, ok := f.file.(fs.ReadDirFile); ok {
		return readDirFile.ReadDir(n)
	}
	return nil, wrapper.PathErr("readdir", f.name, errors.New("not implemented"))
}

func (f *fsFile) Readdir(n int) ([]fs.FileInfo, error) {
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

func (f *fsFile) Readdirnames(n int) (names []string, err error) {
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

func (f *fsFile) Seek(offset int64, whence int) (ret int64, err error) {
	if s, ok := f.file.(io.Seeker); ok {
		return s.Seek(offset, whence)
	}
	return 0, wrapper.PathErr("seek", f.name, ErrOpNotSupported)
}

func (f *fsFile) Stat() (fs.FileInfo, error) {
	return f.file.Stat()
}

func (f *fsFile) Sync() error {
	return f.pathErr("sync")
}

func (f *fsFile) Truncate(size int64) error {
	return f.pathErr("truncate")
}

func (f *fsFile) Write(b []byte) (n int, err error) {
	return 0, f.pathErr("write")
}

func (f *fsFile) WriteAt(b []byte, off int64) (n int, err error) {
	return 0, f.pathErr("write")
}

func (f *fsFile) WriteString(s string) (n int, err error) {
	return 0, f.pathErr("write")
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
// return error that satisfies, for methods on [Fs], errors.Is(err, syscall.EROFS)
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
	return wrapper.PathErr("chmod", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Chown(name string, uid int, gid int) error {
	return wrapper.PathErr("chown", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return wrapper.PathErr("chtimes", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Close() error {
	return nil
}

func (f *ioFsFromUnrooted) Create(name string) (File, error) {
	return nil, wrapper.PathErr("open", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Lchown(name string, uid int, gid int) error {
	return wrapper.PathErr("lchown", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Link(oldname string, newname string) error {
	return wrapper.LinkErr("link", oldname, newname, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Lstat(name string) (fs.FileInfo, error) {
	path, err := f.resolvePath(name)
	if err != nil {
		return nil, wrapper.PathErr("lstat", name, err)
	}
	return f.fsys.Lstat(path)
}

func (f *ioFsFromUnrooted) Mkdir(name string, perm fs.FileMode) error {
	return wrapper.PathErr("mkdir", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) MkdirAll(name string, perm fs.FileMode) error {
	return wrapper.PathErr("mkdir", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Name() string {
	return f.name
}

func (f *ioFsFromUnrooted) Open(name string) (File, error) {
	path, err := f.resolvePath(name)
	if err != nil {
		return nil, wrapper.PathErr("open", name, err)
	}
	file, err := f.fsys.Open(path)
	if err != nil {
		return nil, err
	}
	return newFsFile(file, path), nil
}

func (f *ioFsFromUnrooted) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	if flag&(os.O_WRONLY|syscall.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, wrapper.PathErr("open", name, syscall.EROFS)
	}
	return f.Open(name)
}

func (f *ioFsFromUnrooted) OpenRoot(name string) (Rooted, error) {
	subPath, err := f.resolvePath(name)
	if err != nil {
		return nil, wrapper.PathErr("open", name, err)
	}

	subFsys, err := fs.Sub(f.fsys, subPath)
	if err != nil {
		return nil, err
	}

	readLinkFsys, ok := subFsys.(fs.ReadLinkFS)
	if !ok {
		return nil, fmt.Errorf("*FsUnrooted.OpenRoot: sub fsys does not implement fs.ReadLinkFS")
	}

	return FromIoFsRooted(readLinkFsys, path.Join(f.name, name)), nil
}

func (f *ioFsFromUnrooted) ReadLink(name string) (string, error) {
	resolved, err := f.resolvePath(name)
	if err != nil {
		return "", wrapper.PathErr("readlink", name, err)
	}
	return f.fsys.ReadLink(resolved)
}

func (f *ioFsFromUnrooted) Remove(name string) error {
	return wrapper.PathErr("remove", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) RemoveAll(name string) error {
	return wrapper.PathErr("RemoveAll", name, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Rename(oldname string, newname string) error {
	return wrapper.LinkErr("rename", oldname, newname, syscall.EROFS)
}

func (f *ioFsFromUnrooted) Stat(name string) (fs.FileInfo, error) {
	path, err := f.resolvePath(name)
	if err != nil {
		return nil, wrapper.PathErr("stat", name, err)
	}
	return fs.Stat(f.fsys, path)
}

func (f *ioFsFromUnrooted) Symlink(oldname string, newname string) error {
	return wrapper.LinkErr("symlink", oldname, newname, syscall.EROFS)
}

func (f *ioFsFromUnrooted) OpenUnrooted(name string) (Unrooted, error) {
	subPath, err := f.resolvePath(name)
	if err != nil {
		return nil, wrapper.PathErr("open", name, err)
	}

	subFsys, err := fs.Sub(f.fsys, subPath)
	if err != nil {
		return nil, err
	}

	readLinkFsys, ok := subFsys.(fs.ReadLinkFS)
	if !ok {
		return nil, fmt.Errorf("*FsUnrooted.OpenUnrooted: sub fsys does not implement fs.ReadLinkFS")
	}

	return FromIoFsUnrooted(readLinkFsys, path.Join(f.name, name)), nil
}
