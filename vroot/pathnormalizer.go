package vroot

import (
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

var (
	_ Fs[File] = (*PathNormalizerFs[File])(nil)
	_ Fs[File] = (*PathLocalizerFs[File])(nil)
)

// Compile-time checks for the Root wrappers; written as a generic function so
// the recursive R parameter resolves without picking a concrete root impl.
func _[F File, R Root[F, R]]() {
	var _ Root[F, *PathNormalizerRoot[F, R]] = (*PathNormalizerRoot[F, R])(nil)
	var _ Root[F, *PathLocalizerRoot[F, R]] = (*PathLocalizerRoot[F, R])(nil)
}

func cleanToSlash(p string) string {
	p = filepath.Clean(p)
	p = filepath.ToSlash(p)
	return strings.TrimPrefix(p, "./")
}

func cleanFromSlash(p string) string {
	p = filepath.Clean(p)
	p = filepath.FromSlash(p)
	return strings.TrimPrefix(p, "."+string(filepath.Separator))
}

// PathNormalizerFs wraps an [Fs] whose underlying methods expect [fs.ValidPath]
// (forward-slash) form and presents an Fs that accepts platform-specific
// paths. Each input path is normalized via [filepath.Clean] and converted to
// forward slashes before being forwarded.
//
// Symlink target (oldname) is forwarded unchanged because symlink targets are
// opaque strings stored on the filesystem and not interpreted by the wrapper.
type PathNormalizerFs[F File] struct {
	inner Fs[F]
}

func NewPathNormalizerFs[F File](inner Fs[F]) *PathNormalizerFs[F] {
	return &PathNormalizerFs[F]{inner: inner}
}

func (w *PathNormalizerFs[F]) Chmod(name string, mode fs.FileMode) error {
	return w.inner.Chmod(cleanToSlash(name), mode)
}

func (w *PathNormalizerFs[F]) Chown(name string, uid int, gid int) error {
	return w.inner.Chown(cleanToSlash(name), uid, gid)
}

func (w *PathNormalizerFs[F]) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return w.inner.Chtimes(cleanToSlash(name), atime, mtime)
}

func (w *PathNormalizerFs[F]) Close() error {
	return w.inner.Close()
}

func (w *PathNormalizerFs[F]) Create(name string) (F, error) {
	return w.inner.Create(cleanToSlash(name))
}

func (w *PathNormalizerFs[F]) Lchown(name string, uid int, gid int) error {
	return w.inner.Lchown(cleanToSlash(name), uid, gid)
}

func (w *PathNormalizerFs[F]) Link(oldname string, newname string) error {
	return w.inner.Link(cleanToSlash(oldname), cleanToSlash(newname))
}

func (w *PathNormalizerFs[F]) Lstat(name string) (fs.FileInfo, error) {
	return w.inner.Lstat(cleanToSlash(name))
}

func (w *PathNormalizerFs[F]) Mkdir(name string, perm fs.FileMode) error {
	return w.inner.Mkdir(cleanToSlash(name), perm)
}

func (w *PathNormalizerFs[F]) MkdirAll(name string, perm fs.FileMode) error {
	return w.inner.MkdirAll(cleanToSlash(name), perm)
}

func (w *PathNormalizerFs[F]) Name() string {
	return w.inner.Name()
}

func (w *PathNormalizerFs[F]) Open(name string) (F, error) {
	return w.inner.Open(cleanToSlash(name))
}

func (w *PathNormalizerFs[F]) OpenFile(name string, flag int, perm fs.FileMode) (F, error) {
	return w.inner.OpenFile(cleanToSlash(name), flag, perm)
}

func (w *PathNormalizerFs[F]) ReadLink(name string) (string, error) {
	return w.inner.ReadLink(cleanToSlash(name))
}

func (w *PathNormalizerFs[F]) Remove(name string) error {
	return w.inner.Remove(cleanToSlash(name))
}

func (w *PathNormalizerFs[F]) RemoveAll(name string) error {
	return w.inner.RemoveAll(cleanToSlash(name))
}

func (w *PathNormalizerFs[F]) Rename(oldname string, newname string) error {
	return w.inner.Rename(cleanToSlash(oldname), cleanToSlash(newname))
}

func (w *PathNormalizerFs[F]) Stat(name string) (fs.FileInfo, error) {
	return w.inner.Stat(cleanToSlash(name))
}

func (w *PathNormalizerFs[F]) Symlink(oldname string, newname string) error {
	return w.inner.Symlink(oldname, cleanToSlash(newname))
}

// PathNormalizerRoot is the [Root]-flavored counterpart of [PathNormalizerFs].
// OpenRoot returns another *PathNormalizerRoot so sub-roots stay path-converted.
type PathNormalizerRoot[F File, R Root[F, R]] struct {
	PathNormalizerFs[F]
	rooted R
}

func NewPathNormalizerRoot[F File, R Root[F, R]](inner R) *PathNormalizerRoot[F, R] {
	return &PathNormalizerRoot[F, R]{
		PathNormalizerFs: PathNormalizerFs[F]{inner: inner},
		rooted:           inner,
	}
}

func (w *PathNormalizerRoot[F, R]) IsRoot() {}

func (w *PathNormalizerRoot[F, R]) OpenRoot(name string) (*PathNormalizerRoot[F, R], error) {
	rooted, err := w.rooted.OpenRoot(cleanToSlash(name))
	if err != nil {
		return nil, err
	}
	return NewPathNormalizerRoot(rooted), nil
}

// PathLocalizerFs wraps an [Fs] whose underlying methods expect platform-specific
// paths and presents an Fs that accepts [fs.ValidPath] (forward-slash) form.
// Each input path is normalized via [filepath.Clean] and converted to the
// platform separator before being forwarded.
//
// Symlink target (oldname) is forwarded unchanged.
type PathLocalizerFs[F File] struct {
	inner Fs[F]
}

func NewPathLocalizerFs[F File](inner Fs[F]) *PathLocalizerFs[F] {
	return &PathLocalizerFs[F]{inner: inner}
}

func (w *PathLocalizerFs[F]) Chmod(name string, mode fs.FileMode) error {
	return w.inner.Chmod(cleanFromSlash(name), mode)
}

func (w *PathLocalizerFs[F]) Chown(name string, uid int, gid int) error {
	return w.inner.Chown(cleanFromSlash(name), uid, gid)
}

func (w *PathLocalizerFs[F]) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return w.inner.Chtimes(cleanFromSlash(name), atime, mtime)
}

func (w *PathLocalizerFs[F]) Close() error {
	return w.inner.Close()
}

func (w *PathLocalizerFs[F]) Create(name string) (F, error) {
	return w.inner.Create(cleanFromSlash(name))
}

func (w *PathLocalizerFs[F]) Lchown(name string, uid int, gid int) error {
	return w.inner.Lchown(cleanFromSlash(name), uid, gid)
}

func (w *PathLocalizerFs[F]) Link(oldname string, newname string) error {
	return w.inner.Link(cleanFromSlash(oldname), cleanFromSlash(newname))
}

func (w *PathLocalizerFs[F]) Lstat(name string) (fs.FileInfo, error) {
	return w.inner.Lstat(cleanFromSlash(name))
}

func (w *PathLocalizerFs[F]) Mkdir(name string, perm fs.FileMode) error {
	return w.inner.Mkdir(cleanFromSlash(name), perm)
}

func (w *PathLocalizerFs[F]) MkdirAll(name string, perm fs.FileMode) error {
	return w.inner.MkdirAll(cleanFromSlash(name), perm)
}

func (w *PathLocalizerFs[F]) Name() string {
	return w.inner.Name()
}

func (w *PathLocalizerFs[F]) Open(name string) (F, error) {
	return w.inner.Open(cleanFromSlash(name))
}

func (w *PathLocalizerFs[F]) OpenFile(name string, flag int, perm fs.FileMode) (F, error) {
	return w.inner.OpenFile(cleanFromSlash(name), flag, perm)
}

func (w *PathLocalizerFs[F]) ReadLink(name string) (string, error) {
	return w.inner.ReadLink(cleanFromSlash(name))
}

func (w *PathLocalizerFs[F]) Remove(name string) error {
	return w.inner.Remove(cleanFromSlash(name))
}

func (w *PathLocalizerFs[F]) RemoveAll(name string) error {
	return w.inner.RemoveAll(cleanFromSlash(name))
}

func (w *PathLocalizerFs[F]) Rename(oldname string, newname string) error {
	return w.inner.Rename(cleanFromSlash(oldname), cleanFromSlash(newname))
}

func (w *PathLocalizerFs[F]) Stat(name string) (fs.FileInfo, error) {
	return w.inner.Stat(cleanFromSlash(name))
}

func (w *PathLocalizerFs[F]) Symlink(oldname string, newname string) error {
	return w.inner.Symlink(oldname, cleanFromSlash(newname))
}

// PathLocalizerRoot is the [Root]-flavored counterpart of [PathLocalizerFs].
type PathLocalizerRoot[F File, R Root[F, R]] struct {
	PathLocalizerFs[F]
	rooted R
}

func NewPathLocalizerRoot[F File, R Root[F, R]](inner R) *PathLocalizerRoot[F, R] {
	return &PathLocalizerRoot[F, R]{
		PathLocalizerFs: PathLocalizerFs[F]{inner: inner},
		rooted:          inner,
	}
}

func (w *PathLocalizerRoot[F, R]) IsRoot() {}

func (w *PathLocalizerRoot[F, R]) OpenRoot(name string) (*PathLocalizerRoot[F, R], error) {
	rooted, err := w.rooted.OpenRoot(cleanFromSlash(name))
	if err != nil {
		return nil, err
	}
	return NewPathLocalizerRoot(rooted), nil
}
