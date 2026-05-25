package vroot

import (
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
)

var (
	_ Fs[File] = (*ioFsAsFs)(nil)
	_ File     = (*expandedFile)(nil)
)

// ioFsAsFs adapts an [fs.ReadLinkFS] to a read-only [Fs]. Write operations
// (Chmod, Create, Remove, …) fail with errdef.EROFS. Path traversal via ".."
// or an absolute path is rejected with [ErrPathEscapes], but this is not a
// containment boundary: a symlink whose target points outside the wrapped FS
// is honored verbatim.
//
// No confined Root variant is provided. fs.FS exposes no openat-style
// primitive, so confinement against symlink targets could only be emulated by
// a TOCTOU-prone sequence of Lstat/ReadLink calls — a guarantee this package
// declines to make falsely.
//
// External callers use OS-style paths; the wrapper converts them to
// fs.ValidPath (forward slash) before calling into the underlying fs.FS.
type ioFsAsFs struct {
	fsys fs.ReadLinkFS
	name string
}

// FromIoFs wraps fsys as a read-only [Fs]. name is returned by Name.
func FromIoFs(fsys fs.ReadLinkFS, name string) Fs[File] {
	return &ioFsAsFs{fsys: fsys, name: name}
}

func (f *ioFsAsFs) resolvePath(name string) (string, error) {
	name = filepath.Clean(name)
	if !filepath.IsLocal(name) {
		return "", ErrPathEscapes
	}
	return cleanToSlash(name), nil
}

func (f *ioFsAsFs) Chmod(name string, mode fs.FileMode) error {
	return fsutil.WrapPathErr("chmod", name, errdef.EROFS)
}

func (f *ioFsAsFs) Chown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("chown", name, errdef.EROFS)
}

func (f *ioFsAsFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return fsutil.WrapPathErr("chtimes", name, errdef.EROFS)
}

func (f *ioFsAsFs) Close() error {
	return nil
}

func (f *ioFsAsFs) Create(name string) (File, error) {
	return nil, fsutil.WrapPathErr("open", name, errdef.EROFS)
}

func (f *ioFsAsFs) Lchown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("lchown", name, errdef.EROFS)
}

func (f *ioFsAsFs) Link(oldname string, newname string) error {
	return fsutil.WrapLinkErr("link", oldname, newname, errdef.EROFS)
}

func (f *ioFsAsFs) Lstat(name string) (fs.FileInfo, error) {
	p, err := f.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("lstat", name, err)
	}
	return f.fsys.Lstat(p)
}

func (f *ioFsAsFs) Mkdir(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, errdef.EROFS)
}

func (f *ioFsAsFs) MkdirAll(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, errdef.EROFS)
}

func (f *ioFsAsFs) Name() string {
	return f.name
}

func (f *ioFsAsFs) Open(name string) (File, error) {
	p, err := f.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	file, err := f.fsys.Open(p)
	if err != nil {
		return nil, err
	}
	return ExpandFsFile(file, p), nil
}

func (f *ioFsAsFs) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	if openflag.WriteOp(flag) {
		return nil, fsutil.WrapPathErr("open", name, errdef.EROFS)
	}
	return f.Open(name)
}

func (f *ioFsAsFs) ReadLink(name string) (string, error) {
	p, err := f.resolvePath(name)
	if err != nil {
		return "", fsutil.WrapPathErr("readlink", name, err)
	}
	s, err := f.fsys.ReadLink(p)
	if err != nil {
		return "", err
	}
	return filepath.FromSlash(s), nil
}

func (f *ioFsAsFs) Remove(name string) error {
	return fsutil.WrapPathErr("remove", name, errdef.EROFS)
}

func (f *ioFsAsFs) RemoveAll(name string) error {
	return fsutil.WrapPathErr("RemoveAll", name, errdef.EROFS)
}

func (f *ioFsAsFs) Rename(oldname string, newname string) error {
	return fsutil.WrapLinkErr("rename", oldname, newname, errdef.EROFS)
}

func (f *ioFsAsFs) Stat(name string) (fs.FileInfo, error) {
	p, err := f.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("stat", name, err)
	}
	return fs.Stat(f.fsys, p)
}

func (f *ioFsAsFs) Symlink(oldname string, newname string) error {
	return fsutil.WrapLinkErr("symlink", oldname, newname, errdef.EROFS)
}

// expandedFile widens an [fs.File] into a [vroot.File]. Capabilities beyond
// the basic fs.File contract (ReadAt, Seek, ReadDir) are detected via type
// assertion; unavailable ones return [ErrOpNotSupported]. Writes always fail
// with [syscall.EPERM] — the wrapped FS is treated as read-only.
type expandedFile struct {
	file fs.File
	name string
}

// ExpandFsFile widens file to [vroot.File]. name should be the path that
// resolves to file inside the wrapped FS; it is reported by the File's Name
// method and embedded in error paths.
func ExpandFsFile(file fs.File, name string) File {
	return &expandedFile{file: file, name: name}
}

func (f *expandedFile) pathErr(op string) error {
	return fsutil.WrapPathErr(op, f.name, syscall.EPERM)
}

func (f *expandedFile) Chmod(mode fs.FileMode) error {
	return f.pathErr("chmod")
}

func (f *expandedFile) Chown(uid int, gid int) error {
	return f.pathErr("chown")
}

func (f *expandedFile) Close() error {
	return f.file.Close()
}

func (f *expandedFile) Name() string {
	return filepath.FromSlash(f.name)
}

func (f *expandedFile) Fd() uintptr {
	return Fd(f.file)
}

func (f *expandedFile) Read(b []byte) (n int, err error) {
	return f.file.Read(b)
}

func (f *expandedFile) ReadAt(b []byte, off int64) (n int, err error) {
	if ra, ok := f.file.(io.ReaderAt); ok {
		return ra.ReadAt(b, off)
	}
	return 0, fsutil.WrapPathErr("readat", f.name, ErrOpNotSupported)
}

func (f *expandedFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if readDirFile, ok := f.file.(fs.ReadDirFile); ok {
		return readDirFile.ReadDir(n)
	}
	return nil, fsutil.WrapPathErr("readdir", f.name, errors.New("not implemented"))
}

func (f *expandedFile) Readdir(n int) ([]fs.FileInfo, error) {
	entries, err := f.ReadDir(n)
	if err != nil {
		return nil, err
	}

	infos := make([]fs.FileInfo, len(entries))
	for i, entry := range entries {
		info, err := entry.Info()
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			// Info ≈ Lstat; a concurrent removal between ReadDir and Info is
			// tolerated by suppressing fs.ErrNotExist, mirroring how
			// os.(*File).Readdir behaves with disappearing entries.
			return nil, err
		}
		infos[i] = info
	}
	return infos, nil
}

func (f *expandedFile) Readdirnames(n int) (names []string, err error) {
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

func (f *expandedFile) Seek(offset int64, whence int) (ret int64, err error) {
	if s, ok := f.file.(io.Seeker); ok {
		return s.Seek(offset, whence)
	}
	return 0, fsutil.WrapPathErr("seek", f.name, ErrOpNotSupported)
}

func (f *expandedFile) Stat() (fs.FileInfo, error) {
	return f.file.Stat()
}

func (f *expandedFile) Sync() error {
	return f.pathErr("sync")
}

func (f *expandedFile) Truncate(size int64) error {
	return f.pathErr("truncate")
}

func (f *expandedFile) Write(b []byte) (n int, err error) {
	return 0, f.pathErr("write")
}

func (f *expandedFile) WriteAt(b []byte, off int64) (n int, err error) {
	return 0, f.pathErr("write")
}

func (f *expandedFile) WriteString(s string) (n int, err error) {
	return 0, f.pathErr("write")
}
