package vroot

import (
	"io/fs"
	"time"
)

var (
	_ Fs[File]         = widenedFs[File]{}
	_ ReadDirFs[File]  = widenedFs[File]{}
	_ ReadFileFs[File] = widenedFs[File]{}
	_ SubFs[File]      = widenedFs[File]{}
)

// Widen adapts an [Fs] returning a concrete file type (e.g. *os.File
// from osfs) to the type-erased Fs[File], so implementations with different
// file types can be stored behind a single interface type. Go interfaces are
// not covariant in method return types: Fs[*os.File] does not satisfy
// Fs[File] directly even though *os.File implements [File]. Widen bridges
// that gap by widening the concrete file type parameter F to the [File]
// interface.
//
// If fsys already satisfies Fs[File] it is returned as is. Otherwise the
// returned Fs delegates every call to fsys, converting returned files to
// [File]. It also implements [ReadDirFs], [ReadFileFs] and [SubFs],
// forwarding to fsys's native implementations when present (via [ReadDir],
// [ReadFile] and [Sub]).
func Widen[F File](fsys Fs[F]) Fs[File] {
	if wide, ok := any(fsys).(Fs[File]); ok {
		return wide
	}
	return widenedFs[F]{inner: fsys}
}

type widenedFs[F File] struct {
	inner Fs[F]
}

func (w widenedFs[F]) Chmod(name string, mode fs.FileMode) error {
	return w.inner.Chmod(name, mode)
}

func (w widenedFs[F]) Chown(name string, uid int, gid int) error {
	return w.inner.Chown(name, uid, gid)
}

func (w widenedFs[F]) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return w.inner.Chtimes(name, atime, mtime)
}

func (w widenedFs[F]) Close() error {
	return w.inner.Close()
}

func (w widenedFs[F]) Create(name string) (File, error) {
	f, err := w.inner.Create(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (w widenedFs[F]) Lchown(name string, uid int, gid int) error {
	return w.inner.Lchown(name, uid, gid)
}

func (w widenedFs[F]) Link(oldname string, newname string) error {
	return w.inner.Link(oldname, newname)
}

func (w widenedFs[F]) Lstat(name string) (fs.FileInfo, error) {
	return w.inner.Lstat(name)
}

func (w widenedFs[F]) Mkdir(name string, perm fs.FileMode) error {
	return w.inner.Mkdir(name, perm)
}

func (w widenedFs[F]) MkdirAll(name string, perm fs.FileMode) error {
	return w.inner.MkdirAll(name, perm)
}

func (w widenedFs[F]) Name() string {
	return w.inner.Name()
}

func (w widenedFs[F]) Open(name string) (File, error) {
	f, err := w.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (w widenedFs[F]) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	f, err := w.inner.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (w widenedFs[F]) ReadLink(name string) (string, error) {
	return w.inner.ReadLink(name)
}

func (w widenedFs[F]) Remove(name string) error {
	return w.inner.Remove(name)
}

func (w widenedFs[F]) RemoveAll(name string) error {
	return w.inner.RemoveAll(name)
}

func (w widenedFs[F]) Rename(oldname string, newname string) error {
	return w.inner.Rename(oldname, newname)
}

func (w widenedFs[F]) Stat(name string) (fs.FileInfo, error) {
	return w.inner.Stat(name)
}

func (w widenedFs[F]) Symlink(oldname string, newname string) error {
	return w.inner.Symlink(oldname, newname)
}

// ReadDir implements [ReadDirFs], forwarding to the inner Fs's native
// ReadDir when present.
func (w widenedFs[F]) ReadDir(name string) ([]fs.DirEntry, error) {
	return ReadDir(w.inner, name)
}

// ReadFile implements [ReadFileFs], forwarding to the inner Fs's native
// ReadFile when present.
func (w widenedFs[F]) ReadFile(name string) ([]byte, error) {
	return ReadFile(w.inner, name)
}

// Sub implements [SubFs]. The returned sub-filesystem is widened as well.
func (w widenedFs[F]) Sub(dir string) (Fs[File], error) {
	sub, err := Sub(w.inner, dir)
	if err != nil {
		return nil, err
	}
	return Widen(sub), nil
}
