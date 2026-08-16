package vroot

import (
	"io/fs"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
)

var (
	_ Fs[*ReadOnlyFile] = (*ReadOnlyFs[File])(nil)
	_ File              = (*ReadOnlyFile)(nil)
)

// Compile-time check that *ReadOnlyRoot[F, R] satisfies
// Root[*ReadOnlyFile, *ReadOnlyRoot[F, R]] for all F, R. Written as a generic
// function because the self-referential R parameter cannot be expressed by a
// non-generic var assertion.
func _[F File, R Root[F, R]]() {
	var _ Root[*ReadOnlyFile, *ReadOnlyRoot[F, R]] = (*ReadOnlyRoot[F, R])(nil)
}

// ReadOnlyFs wraps an [Fs] and rejects every write operation with
// [errdef.EROFS]. Reads are forwarded to the wrapped Fs, and files are
// returned as [*ReadOnlyFile] so writes through the file handle are also
// rejected.
type ReadOnlyFs[F File] struct {
	inner Fs[F]
}

func NewReadOnlyFs[F File](inner Fs[F]) *ReadOnlyFs[F] {
	return &ReadOnlyFs[F]{inner: inner}
}

func (r *ReadOnlyFs[F]) Chmod(name string, mode fs.FileMode) error {
	return fsutil.WrapPathErr("chmod", name, errdef.EROFS)
}

func (r *ReadOnlyFs[F]) Chown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("chown", name, errdef.EROFS)
}

func (r *ReadOnlyFs[F]) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return fsutil.WrapPathErr("chtimes", name, errdef.EROFS)
}

func (r *ReadOnlyFs[F]) Close() error {
	return r.inner.Close()
}

func (r *ReadOnlyFs[F]) Create(name string) (*ReadOnlyFile, error) {
	return nil, fsutil.WrapPathErr("open", name, errdef.EROFS)
}

func (r *ReadOnlyFs[F]) Lchown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("lchown", name, errdef.EROFS)
}

func (r *ReadOnlyFs[F]) Link(oldname string, newname string) error {
	return fsutil.WrapLinkErr("link", oldname, newname, errdef.EROFS)
}

func (r *ReadOnlyFs[F]) Lstat(name string) (fs.FileInfo, error) {
	return r.inner.Lstat(name)
}

func (r *ReadOnlyFs[F]) Mkdir(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, errdef.EROFS)
}

func (r *ReadOnlyFs[F]) MkdirAll(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, errdef.EROFS)
}

func (r *ReadOnlyFs[F]) Name() string {
	return r.inner.Name()
}

func (r *ReadOnlyFs[F]) Open(name string) (*ReadOnlyFile, error) {
	return NewReadOnlyFile(r.inner.Open(name))
}

func (r *ReadOnlyFs[F]) OpenFile(name string, flag int, perm fs.FileMode) (*ReadOnlyFile, error) {
	if openflag.WriteOp(flag) {
		return nil, fsutil.WrapPathErr("open", name, errdef.EROFS)
	}
	return NewReadOnlyFile(r.inner.OpenFile(name, flag, perm))
}

func (r *ReadOnlyFs[F]) ReadLink(name string) (string, error) {
	return r.inner.ReadLink(name)
}

func (r *ReadOnlyFs[F]) Remove(name string) error {
	return fsutil.WrapPathErr("remove", name, errdef.EROFS)
}

func (r *ReadOnlyFs[F]) RemoveAll(name string) error {
	return fsutil.WrapPathErr("RemoveAll", name, errdef.EROFS)
}

func (r *ReadOnlyFs[F]) Rename(oldname string, newname string) error {
	return fsutil.WrapLinkErr("rename", oldname, newname, errdef.EROFS)
}

func (r *ReadOnlyFs[F]) Stat(name string) (fs.FileInfo, error) {
	return r.inner.Stat(name)
}

func (r *ReadOnlyFs[F]) Symlink(oldname string, newname string) error {
	return fsutil.WrapLinkErr("symlink", oldname, newname, errdef.EROFS)
}

// ReadOnlyRoot wraps a [Root] and rejects every write operation with
// [errdef.EROFS]. OpenRoot returns another *ReadOnlyRoot so sub-roots stay
// read-only.
type ReadOnlyRoot[F File, R Root[F, R]] struct {
	inner R
}

func NewReadOnlyRoot[F File, R Root[F, R]](inner R) *ReadOnlyRoot[F, R] {
	return &ReadOnlyRoot[F, R]{inner: inner}
}

func (r *ReadOnlyRoot[F, R]) IsRoot() {}

func (r *ReadOnlyRoot[F, R]) Chmod(name string, mode fs.FileMode) error {
	return fsutil.WrapPathErr("chmod", name, errdef.EROFS)
}

func (r *ReadOnlyRoot[F, R]) Chown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("chown", name, errdef.EROFS)
}

func (r *ReadOnlyRoot[F, R]) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return fsutil.WrapPathErr("chtimes", name, errdef.EROFS)
}

func (r *ReadOnlyRoot[F, R]) Close() error {
	return r.inner.Close()
}

func (r *ReadOnlyRoot[F, R]) Create(name string) (*ReadOnlyFile, error) {
	return nil, fsutil.WrapPathErr("open", name, errdef.EROFS)
}

func (r *ReadOnlyRoot[F, R]) Lchown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("lchown", name, errdef.EROFS)
}

func (r *ReadOnlyRoot[F, R]) Link(oldname string, newname string) error {
	return fsutil.WrapLinkErr("link", oldname, newname, errdef.EROFS)
}

func (r *ReadOnlyRoot[F, R]) Lstat(name string) (fs.FileInfo, error) {
	return r.inner.Lstat(name)
}

func (r *ReadOnlyRoot[F, R]) Mkdir(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, errdef.EROFS)
}

func (r *ReadOnlyRoot[F, R]) MkdirAll(name string, perm fs.FileMode) error {
	return fsutil.WrapPathErr("mkdir", name, errdef.EROFS)
}

func (r *ReadOnlyRoot[F, R]) Name() string {
	return r.inner.Name()
}

func (r *ReadOnlyRoot[F, R]) Open(name string) (*ReadOnlyFile, error) {
	return NewReadOnlyFile(r.inner.Open(name))
}

func (r *ReadOnlyRoot[F, R]) OpenFile(
	name string,
	flag int,
	perm fs.FileMode,
) (*ReadOnlyFile, error) {
	if openflag.WriteOp(flag) {
		return nil, fsutil.WrapPathErr("open", name, errdef.EROFS)
	}
	return NewReadOnlyFile(r.inner.OpenFile(name, flag, perm))
}

func (r *ReadOnlyRoot[F, R]) OpenRoot(name string) (*ReadOnlyRoot[F, R], error) {
	rooted, err := r.inner.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return &ReadOnlyRoot[F, R]{inner: rooted}, nil
}

func (r *ReadOnlyRoot[F, R]) ReadLink(name string) (string, error) {
	return r.inner.ReadLink(name)
}

func (r *ReadOnlyRoot[F, R]) Remove(name string) error {
	return fsutil.WrapPathErr("remove", name, errdef.EROFS)
}

func (r *ReadOnlyRoot[F, R]) RemoveAll(name string) error {
	return fsutil.WrapPathErr("RemoveAll", name, errdef.EROFS)
}

func (r *ReadOnlyRoot[F, R]) Rename(oldname string, newname string) error {
	return fsutil.WrapLinkErr("rename", oldname, newname, errdef.EROFS)
}

func (r *ReadOnlyRoot[F, R]) Stat(name string) (fs.FileInfo, error) {
	return r.inner.Stat(name)
}

func (r *ReadOnlyRoot[F, R]) Symlink(oldname string, newname string) error {
	return fsutil.WrapLinkErr("symlink", oldname, newname, errdef.EROFS)
}

// ReadOnlyFile wraps a [File] and rejects every write operation with
// [syscall.EPERM]. Reads are forwarded.
type ReadOnlyFile struct {
	f File
}

// NewReadOnlyFile is shaped to be used as a one-liner with a result of an
// Open-like call: NewReadOnlyFile(fsys.Open(name)).
//
// err is checked first: an Fs whose File type is a pointer (e.g. *os.File)
// returns a typed-nil on failure, which is a non-nil File interface here.
// Testing f == nil would wrap that nil and return a non-nil file alongside
// the error.
func NewReadOnlyFile(f File, err error) (*ReadOnlyFile, error) {
	if err != nil {
		return nil, err
	}
	return &ReadOnlyFile{f: f}, nil
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
