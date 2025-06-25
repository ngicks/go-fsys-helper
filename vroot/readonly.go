package vroot

import (
	"io/fs"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
)

var _ Rooted = (*ReadOnlyRooted)(nil)

type ReadOnlyRooted struct {
	rooted Rooted
}

func NewReadOnlyRooted(rooted Rooted) *ReadOnlyRooted {
	return &ReadOnlyRooted{rooted: rooted}
}

func (r *ReadOnlyRooted) Rooted() {
}

func (r *ReadOnlyRooted) Chmod(name string, mode fs.FileMode) error {
	return fsutil.WrapPathErr("chmod", name, errdef.EROFS)
}

func (r *ReadOnlyRooted) Chown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("chown", name, errdef.EROFS)
}

func (r *ReadOnlyRooted) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return fsutil.WrapPathErr("chtimes", name, errdef.EROFS)
}

func (r *ReadOnlyRooted) Close() error {
	return r.rooted.Close()
}

func (r *ReadOnlyRooted) Create(name string) (File, error) {
	return nil, fsutil.WrapPathErr("open", name, errdef.EROFS)
}

func (r *ReadOnlyRooted) Lchown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("lchown", name, errdef.EROFS)
}

func (r *ReadOnlyRooted) Link(oldname string, newname string) error {
	return fsutil.WrapLinkErr("link", oldname, newname, errdef.EROFS)
}

func (r *ReadOnlyRooted) Lstat(name string) (fs.FileInfo, error) {
	return r.rooted.Lstat(name)
}

func (r *ReadOnlyRooted) Mkdir(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, errdef.EROFS)
}

func (r *ReadOnlyRooted) MkdirAll(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, errdef.EROFS)
}

func (r *ReadOnlyRooted) Name() string {
	return r.rooted.Name()
}

func (r *ReadOnlyRooted) Open(name string) (File, error) {
	return NewReadOnlyFile(r.rooted.Open(name))
}

func (r *ReadOnlyRooted) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	if openflag.WriteOp(flag) {
		return nil, fsutil.WrapPathErr("open", name, errdef.EROFS)
	}
	return NewReadOnlyFile(r.Open(name))
}

func (r *ReadOnlyRooted) OpenRoot(name string) (Rooted, error) {
	rooted, err := r.rooted.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return NewReadOnlyRooted(rooted), nil
}

func (r *ReadOnlyRooted) ReadLink(name string) (string, error) {
	return r.rooted.ReadLink(name)
}

func (r *ReadOnlyRooted) Remove(name string) error {
	return fsutil.WrapPathErr("remove", name, errdef.EROFS)
}

func (r *ReadOnlyRooted) RemoveAll(name string) error {
	return fsutil.WrapPathErr("RemoveAll", name, errdef.EROFS)
}

func (r *ReadOnlyRooted) Rename(oldname string, newname string) error {
	return fsutil.WrapLinkErr("rename", oldname, newname, errdef.EROFS)
}

func (r *ReadOnlyRooted) Stat(name string) (fs.FileInfo, error) {
	return r.rooted.Stat(name)
}

func (r *ReadOnlyRooted) Symlink(oldname string, newname string) error {
	return fsutil.WrapLinkErr("symlink", oldname, newname, errdef.EROFS)
}

var _ Unrooted = (*ReadOnlyUnrooted)(nil)

type ReadOnlyUnrooted struct {
	rooted Unrooted
}

func NewReadOnlyUnrooted(rooted Unrooted) *ReadOnlyUnrooted {
	return &ReadOnlyUnrooted{rooted: rooted}
}

func (r *ReadOnlyUnrooted) Unrooted() {}

func (r *ReadOnlyUnrooted) Chmod(name string, mode fs.FileMode) error {
	return fsutil.WrapPathErr("chmod", name, errdef.EROFS)
}

func (r *ReadOnlyUnrooted) Chown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("chown", name, errdef.EROFS)
}

func (r *ReadOnlyUnrooted) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return fsutil.WrapPathErr("chtimes", name, errdef.EROFS)
}

func (r *ReadOnlyUnrooted) Close() error {
	return r.rooted.Close()
}

func (r *ReadOnlyUnrooted) Create(name string) (File, error) {
	return nil, fsutil.WrapPathErr("open", name, errdef.EROFS)
}

func (r *ReadOnlyUnrooted) Lchown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("lchown", name, errdef.EROFS)
}

func (r *ReadOnlyUnrooted) Link(oldname string, newname string) error {
	return fsutil.WrapLinkErr("link", oldname, newname, errdef.EROFS)
}

func (r *ReadOnlyUnrooted) Lstat(name string) (fs.FileInfo, error) {
	return r.rooted.Lstat(name)
}

func (r *ReadOnlyUnrooted) Mkdir(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, errdef.EROFS)
}

func (r *ReadOnlyUnrooted) MkdirAll(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, errdef.EROFS)
}

func (r *ReadOnlyUnrooted) Name() string {
	return r.rooted.Name()
}

func (r *ReadOnlyUnrooted) Open(name string) (File, error) {
	return NewReadOnlyFile(r.rooted.Open(name))
}

func (r *ReadOnlyUnrooted) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	if openflag.WriteOp(flag) {
		return nil, fsutil.WrapPathErr("open", name, errdef.EROFS)
	}
	return NewReadOnlyFile(r.Open(name))
}

func (r *ReadOnlyUnrooted) OpenRoot(name string) (Rooted, error) {
	rooted, err := r.rooted.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return NewReadOnlyRooted(rooted), nil
}

func (r *ReadOnlyUnrooted) OpenUnrooted(name string) (Unrooted, error) {
	rooted, err := r.rooted.OpenUnrooted(name)
	if err != nil {
		return nil, err
	}
	return NewReadOnlyUnrooted(rooted), nil
}

func (r *ReadOnlyUnrooted) ReadLink(name string) (string, error) {
	return r.rooted.ReadLink(name)
}

func (r *ReadOnlyUnrooted) Remove(name string) error {
	return fsutil.WrapPathErr("remove", name, errdef.EROFS)
}

func (r *ReadOnlyUnrooted) RemoveAll(name string) error {
	return fsutil.WrapPathErr("RemoveAll", name, errdef.EROFS)
}

func (r *ReadOnlyUnrooted) Rename(oldname string, newname string) error {
	return fsutil.WrapLinkErr("rename", oldname, newname, errdef.EROFS)
}

func (r *ReadOnlyUnrooted) Stat(name string) (fs.FileInfo, error) {
	return r.rooted.Stat(name)
}

func (r *ReadOnlyUnrooted) Symlink(oldname string, newname string) error {
	return fsutil.WrapLinkErr("symlink", oldname, newname, errdef.EROFS)
}

var _ File = (*ReadOnlyFile)(nil)

type ReadOnlyFile struct {
	f File
}

func NewReadOnlyFile(f File, err error) (File, error) {
	if f == nil {
		return nil, err
	}
	return &ReadOnlyFile{f: f}, err
}

func (r *ReadOnlyFile) pathErr(op string) error {
	return fsutil.WrapPathErr(op, r.f.Name(), syscall.EPERM)
}

func (r *ReadOnlyFile) Chmod(mode fs.FileMode) error {
	return r.pathErr("chmod")
}

func (r *ReadOnlyFile) Chown(uid int, gid int) error {
	return r.pathErr("chown")
}

func (r *ReadOnlyFile) Close() error {
	return r.f.Close()
}

func (r *ReadOnlyFile) Name() string {
	return r.f.Name()
}

func (r *ReadOnlyFile) Fd() uintptr {
	return r.f.Fd()
}

func (r *ReadOnlyFile) Read(b []byte) (n int, err error) {
	return r.f.Read(b)
}

func (r *ReadOnlyFile) ReadAt(b []byte, off int64) (n int, err error) {
	return r.f.ReadAt(b, off)
}

func (r *ReadOnlyFile) ReadDir(n int) ([]fs.DirEntry, error) {
	return r.f.ReadDir(n)
}

func (r *ReadOnlyFile) Readdir(n int) ([]fs.FileInfo, error) {
	return r.f.Readdir(n)
}

func (r *ReadOnlyFile) Readdirnames(n int) (names []string, err error) {
	return r.f.Readdirnames(n)
}

func (r *ReadOnlyFile) Seek(offset int64, whence int) (ret int64, err error) {
	return r.f.Seek(offset, whence)
}

func (r *ReadOnlyFile) Stat() (fs.FileInfo, error) {
	return r.f.Stat()
}

func (r *ReadOnlyFile) Sync() error {
	return r.pathErr("sync")
}

func (r *ReadOnlyFile) Truncate(size int64) error {
	return r.pathErr("truncate")
}

func (r *ReadOnlyFile) Write(b []byte) (n int, err error) {
	return 0, r.pathErr("write")
}

func (r *ReadOnlyFile) WriteAt(b []byte, off int64) (n int, err error) {
	return 0, r.pathErr("writeat")
}

func (r *ReadOnlyFile) WriteString(s string) (n int, err error) {
	return 0, r.pathErr("write")
}
