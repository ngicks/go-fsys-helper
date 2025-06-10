package vroot

import (
	"io/fs"
	"os"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot/internal/wrapper"
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
	return wrapper.PathErr("chmod", name, syscall.EROFS)
}

func (r *ReadOnlyRooted) Chown(name string, uid int, gid int) error {
	return wrapper.PathErr("chown", name, syscall.EROFS)
}

func (r *ReadOnlyRooted) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return wrapper.PathErr("chtimes", name, syscall.EROFS)
}

func (r *ReadOnlyRooted) Close() error {
	return nil
}

func (r *ReadOnlyRooted) Create(name string) (File, error) {
	return nil, wrapper.PathErr("open", name, syscall.EROFS)
}

func (r *ReadOnlyRooted) Lchown(name string, uid int, gid int) error {
	return wrapper.PathErr("lchown", name, syscall.EROFS)
}

func (r *ReadOnlyRooted) Link(oldname string, newname string) error {
	return wrapper.LinkErr("link", oldname, newname, syscall.EROFS)
}

func (r *ReadOnlyRooted) Lstat(name string) (fs.FileInfo, error) {
	return r.rooted.Lstat(name)
}

func (r *ReadOnlyRooted) Mkdir(name string, perm fs.FileMode) error {
	return wrapper.PathErr("mkdir", name, syscall.EROFS)
}

func (r *ReadOnlyRooted) MkdirAll(name string, perm fs.FileMode) error {
	return wrapper.PathErr("mkdir", name, syscall.EROFS)
}

func (r *ReadOnlyRooted) Name() string {
	return r.rooted.Name()
}

func (r *ReadOnlyRooted) Open(name string) (File, error) {
	return newReadOnlyFile(r.rooted.Open(name))
}

func (r *ReadOnlyRooted) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	if flag&(os.O_WRONLY|syscall.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, wrapper.PathErr("open", name, syscall.EROFS)
	}
	return newReadOnlyFile(r.Open(name))
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
	return wrapper.PathErr("remove", name, syscall.EROFS)
}

func (r *ReadOnlyRooted) RemoveAll(name string) error {
	return wrapper.PathErr("RemoveAll", name, syscall.EROFS)
}

func (r *ReadOnlyRooted) Rename(oldname string, newname string) error {
	return wrapper.LinkErr("rename", oldname, newname, syscall.EROFS)
}

func (r *ReadOnlyRooted) Stat(name string) (fs.FileInfo, error) {
	return r.rooted.Stat(name)
}

func (r *ReadOnlyRooted) Symlink(oldname string, newname string) error {
	return wrapper.LinkErr("symlink", oldname, newname, syscall.EROFS)
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
	return wrapper.PathErr("chmod", name, syscall.EROFS)
}

func (r *ReadOnlyUnrooted) Chown(name string, uid int, gid int) error {
	return wrapper.PathErr("chown", name, syscall.EROFS)
}

func (r *ReadOnlyUnrooted) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return wrapper.PathErr("chtimes", name, syscall.EROFS)
}

func (r *ReadOnlyUnrooted) Close() error {
	return nil
}

func (r *ReadOnlyUnrooted) Create(name string) (File, error) {
	return nil, wrapper.PathErr("open", name, syscall.EROFS)
}

func (r *ReadOnlyUnrooted) Lchown(name string, uid int, gid int) error {
	return wrapper.PathErr("lchown", name, syscall.EROFS)
}

func (r *ReadOnlyUnrooted) Link(oldname string, newname string) error {
	return wrapper.LinkErr("link", oldname, newname, syscall.EROFS)
}

func (r *ReadOnlyUnrooted) Lstat(name string) (fs.FileInfo, error) {
	return r.rooted.Lstat(name)
}

func (r *ReadOnlyUnrooted) Mkdir(name string, perm fs.FileMode) error {
	return wrapper.PathErr("mkdir", name, syscall.EROFS)
}

func (r *ReadOnlyUnrooted) MkdirAll(name string, perm fs.FileMode) error {
	return wrapper.PathErr("mkdir", name, syscall.EROFS)
}

func (r *ReadOnlyUnrooted) Name() string {
	return r.rooted.Name()
}

func (r *ReadOnlyUnrooted) Open(name string) (File, error) {
	return newReadOnlyFile(r.rooted.Open(name))
}

func (r *ReadOnlyUnrooted) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	if flag&(os.O_WRONLY|syscall.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, wrapper.PathErr("open", name, syscall.EROFS)
	}
	return newReadOnlyFile(r.Open(name))
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
	return wrapper.PathErr("remove", name, syscall.EROFS)
}

func (r *ReadOnlyUnrooted) RemoveAll(name string) error {
	return wrapper.PathErr("RemoveAll", name, syscall.EROFS)
}

func (r *ReadOnlyUnrooted) Rename(oldname string, newname string) error {
	return wrapper.LinkErr("rename", oldname, newname, syscall.EROFS)
}

func (r *ReadOnlyUnrooted) Stat(name string) (fs.FileInfo, error) {
	return r.rooted.Stat(name)
}

func (r *ReadOnlyUnrooted) Symlink(oldname string, newname string) error {
	return wrapper.LinkErr("symlink", oldname, newname, syscall.EROFS)
}

var _ File = (*readOnlyFile)(nil)

type readOnlyFile struct {
	f File
}

func newReadOnlyFile(f File, err error) (File, error) {
	if f == nil {
		return nil, err
	}
	return &readOnlyFile{f: f}, err
}

func (r *readOnlyFile) pathErr(op string) error {
	return wrapper.PathErr(op, r.f.Name(), syscall.EPERM)
}

func (r *readOnlyFile) Chmod(mode fs.FileMode) error {
	return r.pathErr("chmod")
}

func (r *readOnlyFile) Chown(uid int, gid int) error {
	return r.pathErr("chown")
}

func (r *readOnlyFile) Close() error {
	return r.f.Close()
}

func (r *readOnlyFile) Name() string {
	return r.f.Name()
}

func (r *readOnlyFile) Fd() uintptr {
	return r.f.Fd()
}

func (r *readOnlyFile) Read(b []byte) (n int, err error) {
	return r.f.Read(b)
}

func (r *readOnlyFile) ReadAt(b []byte, off int64) (n int, err error) {
	return r.f.ReadAt(b, off)
}

func (r *readOnlyFile) ReadDir(n int) ([]fs.DirEntry, error) {
	return r.f.ReadDir(n)
}

func (r *readOnlyFile) Readdir(n int) ([]fs.FileInfo, error) {
	return r.f.Readdir(n)
}

func (r *readOnlyFile) Readdirnames(n int) (names []string, err error) {
	return r.f.Readdirnames(n)
}

func (r *readOnlyFile) Seek(offset int64, whence int) (ret int64, err error) {
	return r.f.Seek(offset, whence)
}

func (r *readOnlyFile) Stat() (fs.FileInfo, error) {
	return r.f.Stat()
}

func (r *readOnlyFile) Sync() error {
	return r.pathErr("sync")
}

func (r *readOnlyFile) Truncate(size int64) error {
	return r.pathErr("truncate")
}

func (r *readOnlyFile) Write(b []byte) (n int, err error) {
	return 0, r.pathErr("write")
}

func (r *readOnlyFile) WriteAt(b []byte, off int64) (n int, err error) {
	return 0, r.pathErr("writeat")
}

func (r *readOnlyFile) WriteString(s string) (n int, err error) {
	return 0, r.pathErr("write")
}
