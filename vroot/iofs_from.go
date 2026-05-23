package vroot

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
)

var (
	_ Fs[File]                = (*ioFsAsFs)(nil)
	_ Root[File, *ioFsAsRoot] = (*ioFsAsRoot)(nil)
	_ File                    = (*expandedFile)(nil)
)

// pathConverter adapts an fs.ReadLinkFS so its Lstat/ReadLink accept OS-style
// paths. fsutil.ResolvePath drives traversal in OS-style form; we route the
// individual lookups back through cleanToSlash so the underlying fs.FS sees
// fs.ValidPath form.
type pathConverter struct {
	fsys fs.ReadLinkFS
}

func (c pathConverter) ReadLink(name string) (string, error) {
	return c.fsys.ReadLink(cleanToSlash(name))
}

func (c pathConverter) Lstat(name string) (fs.FileInfo, error) {
	return c.fsys.Lstat(cleanToSlash(name))
}

// ioFsAsFs adapts an [fs.ReadLinkFS] to a read-only [Fs]. Write operations
// (Chmod, Create, Remove, …) fail with errdef.EROFS. Path traversal via ".."
// or absolute paths is rejected with [ErrPathEscapes]. Symlinks pointing
// outside the wrapped FS are honored verbatim — use [ioFsAsRoot] if you need
// containment enforced against symlink targets.
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

// ioFsAsRoot adapts an [fs.ReadLinkFS] to a read-only [Root]. Stricter than
// [ioFsAsFs]: paths are resolved component-by-component via [fsutil.ResolvePath]
// so any symlink target that would escape the wrapped FS produces
// [ErrPathEscapes]. OpenRoot uses [fs.Sub] on the underlying FS.
//
// Like [ioFsAsFs] it is read-only; write operations fail with errdef.EROFS, and
// it is still vulnerable to TOCTOU races because resolution is a sequence of
// Lstat and ReadLink calls (unlike *os.Root, which uses openat-style APIs).
type ioFsAsRoot struct {
	fsys fs.ReadLinkFS
	name string
}

// FromIoFsRoot wraps fsys as a read-only [Root]. name is returned by Name.
// The concrete return type is unexported; callers bind it with := or assign
// to vroot.[Fs] / vroot.[Root] when the type parameter is otherwise needed.
func FromIoFsRoot(fsys fs.ReadLinkFS, name string) Root[File, *ioFsAsRoot] {
	return &ioFsAsRoot{fsys: fsys, name: name}
}

func (f *ioFsAsRoot) IsRoot() {}

func (f *ioFsAsRoot) resolvePath(name string, skipLastElement bool) (string, error) {
	s, err := fsutil.ResolvePath(pathConverter{f.fsys}, name, skipLastElement)
	return cleanToSlash(s), err
}

func (f *ioFsAsRoot) Chmod(name string, mode fs.FileMode) error {
	return fsutil.WrapPathErr("chmod", name, errdef.EROFS)
}

func (f *ioFsAsRoot) Chown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("chown", name, errdef.EROFS)
}

func (f *ioFsAsRoot) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return fsutil.WrapPathErr("chtimes", name, errdef.EROFS)
}

func (f *ioFsAsRoot) Close() error {
	return nil
}

func (f *ioFsAsRoot) Create(name string) (File, error) {
	return nil, fsutil.WrapPathErr("open", name, errdef.EROFS)
}

func (f *ioFsAsRoot) Lchown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("lchown", name, errdef.EROFS)
}

func (f *ioFsAsRoot) Link(oldname string, newname string) error {
	return fsutil.WrapLinkErr("link", oldname, newname, errdef.EROFS)
}

func (f *ioFsAsRoot) Lstat(name string) (fs.FileInfo, error) {
	p, err := f.resolvePath(name, true)
	if err != nil {
		return nil, fsutil.WrapPathErr("lstat", name, err)
	}
	return f.fsys.Lstat(p)
}

func (f *ioFsAsRoot) Mkdir(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, errdef.EROFS)
}

func (f *ioFsAsRoot) MkdirAll(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, errdef.EROFS)
}

func (f *ioFsAsRoot) Name() string {
	return f.name
}

func (f *ioFsAsRoot) Open(name string) (File, error) {
	p, err := f.resolvePath(name, false)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	file, err := f.fsys.Open(p)
	if err != nil {
		return nil, err
	}
	return ExpandFsFile(file, p), nil
}

func (f *ioFsAsRoot) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	if openflag.WriteOp(flag) {
		return nil, fsutil.WrapPathErr("open", name, errdef.EROFS)
	}
	return f.Open(name)
}

func (f *ioFsAsRoot) OpenRoot(name string) (*ioFsAsRoot, error) {
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
		return nil, fmt.Errorf("*ioFsAsRoot.OpenRoot: sub fsys does not implement fs.ReadLinkFS")
	}

	return &ioFsAsRoot{fsys: readLinkFsys, name: path.Join(f.name, cleanToSlash(name))}, nil
}

func (f *ioFsAsRoot) ReadLink(name string) (string, error) {
	resolved, err := f.resolvePath(name, true)
	if err != nil {
		return "", fsutil.WrapPathErr("readlink", name, err)
	}
	s, err := f.fsys.ReadLink(resolved)
	if err != nil {
		return "", err
	}
	return filepath.FromSlash(s), nil
}

func (f *ioFsAsRoot) Remove(name string) error {
	return fsutil.WrapPathErr("remove", name, errdef.EROFS)
}

func (f *ioFsAsRoot) RemoveAll(name string) error {
	return fsutil.WrapPathErr("RemoveAll", name, errdef.EROFS)
}

func (f *ioFsAsRoot) Rename(oldname string, newname string) error {
	return fsutil.WrapLinkErr("rename", oldname, newname, errdef.EROFS)
}

func (f *ioFsAsRoot) Stat(name string) (fs.FileInfo, error) {
	p, err := f.resolvePath(name, false)
	if err != nil {
		return nil, fsutil.WrapPathErr("stat", name, err)
	}
	return fs.Stat(f.fsys, p)
}

func (f *ioFsAsRoot) Symlink(oldname string, newname string) error {
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
