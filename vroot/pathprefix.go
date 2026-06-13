package vroot

import (
	"io/fs"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
)

var _ Fs[File] = (*PathPrefixFs[File])(nil)

// PathPrefixFs prefixes file paths in every method, effectively creating a sub
// Fs rooted at prefix.
//
// It is NOT a security boundary. Confinement is lexical only: resolvePath blocks
// a ".." path *argument* from climbing above the prefix (via [filepath.IsLocal]),
// but it does NOT confine symlink targets — a symlink stored inside the prefix
// can still resolve to a path outside it when followed by the inner Fs. It is
// also vulnerable to TOCTOU races. For an actual confinement boundary that
// confines symlink targets too, use a [Root] (e.g. [osfs.NewRoot] /
// [Root.OpenRoot]).
type PathPrefixFs[F File] struct {
	inner  Fs[F]
	prefix string
}

// NewPathPrefixFs wraps inner so all paths are resolved relative to prefix.
//
// Like [osfs.NewFs], the prefix is validated up front: it must name an
// existing directory in inner, otherwise the inner Stat error (or
// [syscall.ENOTDIR] for a non-directory) is returned. An empty prefix is
// rejected with [fs.ErrInvalid] — an inner Fs would clean it to "." (its
// root), making the wrapper a no-op.
func NewPathPrefixFs[F File](inner Fs[F], prefix string) (*PathPrefixFs[F], error) {
	if prefix == "" {
		return nil, fsutil.WrapPathErr("stat", prefix, fs.ErrInvalid)
	}
	prefix = filepath.Clean(prefix)
	info, err := inner.Stat(prefix)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fsutil.WrapPathErr("stat", prefix, syscall.ENOTDIR)
	}
	return &PathPrefixFs[F]{inner: inner, prefix: prefix}, nil
}

func (w *PathPrefixFs[F]) resolvePath(p string) (string, error) {
	p = filepath.Clean(p)
	if p == "." {
		return w.prefix, nil
	}
	if !filepath.IsLocal(p) {
		return "", ErrPathEscapes
	}
	return filepath.Join(w.prefix, p), nil
}

func (w *PathPrefixFs[F]) Chmod(name string, mode fs.FileMode) error {
	p, err := w.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("chmod", name, err)
	}
	return w.inner.Chmod(p, mode)
}

func (w *PathPrefixFs[F]) Chown(name string, uid int, gid int) error {
	p, err := w.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("chown", name, err)
	}
	return w.inner.Chown(p, uid, gid)
}

func (w *PathPrefixFs[F]) Chtimes(name string, atime time.Time, mtime time.Time) error {
	p, err := w.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("chtimes", name, err)
	}
	return w.inner.Chtimes(p, atime, mtime)
}

func (w *PathPrefixFs[F]) Close() error {
	return w.inner.Close()
}

func (w *PathPrefixFs[F]) Create(name string) (F, error) {
	var zero F
	p, err := w.resolvePath(name)
	if err != nil {
		return zero, fsutil.WrapPathErr("open", name, err)
	}
	return w.inner.Create(p)
}

func (w *PathPrefixFs[F]) Lchown(name string, uid int, gid int) error {
	p, err := w.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("lchown", name, err)
	}
	return w.inner.Lchown(p, uid, gid)
}

func (w *PathPrefixFs[F]) Link(oldname string, newname string) error {
	oldPath, err := w.resolvePath(oldname)
	if err != nil {
		return fsutil.WrapLinkErr("link", oldname, newname, err)
	}
	newPath, err := w.resolvePath(newname)
	if err != nil {
		return fsutil.WrapLinkErr("link", oldname, newname, err)
	}
	return w.inner.Link(oldPath, newPath)
}

func (w *PathPrefixFs[F]) Lstat(name string) (fs.FileInfo, error) {
	p, err := w.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("lstat", name, err)
	}
	return w.inner.Lstat(p)
}

func (w *PathPrefixFs[F]) Mkdir(name string, perm fs.FileMode) error {
	p, err := w.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("mkdir", name, err)
	}
	return w.inner.Mkdir(p, perm)
}

func (w *PathPrefixFs[F]) MkdirAll(name string, perm fs.FileMode) error {
	p, err := w.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("mkdir", name, err)
	}
	return w.inner.MkdirAll(p, perm)
}

// Name reports the prefix together with the inner Fs's name, in the form
// "prefix=<prefix>: <inner name>".
func (w *PathPrefixFs[F]) Name() string {
	return "prefix=" + w.prefix + ": " + w.inner.Name()
}

func (w *PathPrefixFs[F]) Open(name string) (F, error) {
	var zero F
	p, err := w.resolvePath(name)
	if err != nil {
		return zero, fsutil.WrapPathErr("open", name, err)
	}
	return w.inner.Open(p)
}

func (w *PathPrefixFs[F]) OpenFile(name string, flag int, perm fs.FileMode) (F, error) {
	var zero F
	p, err := w.resolvePath(name)
	if err != nil {
		return zero, fsutil.WrapPathErr("open", name, err)
	}
	return w.inner.OpenFile(p, flag, perm)
}

func (w *PathPrefixFs[F]) ReadLink(name string) (string, error) {
	p, err := w.resolvePath(name)
	if err != nil {
		return "", fsutil.WrapPathErr("readlink", name, err)
	}
	return w.inner.ReadLink(p)
}

func (w *PathPrefixFs[F]) Remove(name string) error {
	p, err := w.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("remove", name, err)
	}
	return w.inner.Remove(p)
}

func (w *PathPrefixFs[F]) RemoveAll(name string) error {
	// Removing the prefixed root itself is refused with EINVAL, matching
	// os.RemoveAll / *os.Root.RemoveAll. Otherwise resolvePath maps "." to the
	// prefix and the call would delete the prefix directory.
	if filepath.Clean(name) == "." {
		return fsutil.WrapPathErr("RemoveAll", name, syscall.EINVAL)
	}
	p, err := w.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("RemoveAll", name, err)
	}
	return w.inner.RemoveAll(p)
}

func (w *PathPrefixFs[F]) Rename(oldname string, newname string) error {
	oldPath, err := w.resolvePath(oldname)
	if err != nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, err)
	}
	newPath, err := w.resolvePath(newname)
	if err != nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, err)
	}
	return w.inner.Rename(oldPath, newPath)
}

func (w *PathPrefixFs[F]) Stat(name string) (fs.FileInfo, error) {
	p, err := w.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("stat", name, err)
	}
	return w.inner.Stat(p)
}

// Symlink only resolves newname against the prefix. The target oldname is
// stored verbatim because symlink targets are opaque strings interpreted by
// the underlying filesystem when followed.
func (w *PathPrefixFs[F]) Symlink(oldname string, newname string) error {
	newPath, err := w.resolvePath(newname)
	if err != nil {
		return fsutil.WrapLinkErr("symlink", oldname, newname, err)
	}
	return w.inner.Symlink(oldname, newPath)
}
