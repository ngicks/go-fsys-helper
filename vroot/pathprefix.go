package vroot

import (
	"io/fs"
	"path/filepath"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
)

var _ Fs[File] = (*PathPrefixFs[File])(nil)

// PathPrefixFs wraps an [Fs] and exposes it as if rooted at the configured
// prefix. Incoming paths are validated with [filepath.IsLocal] (so traversal
// like ".." or an absolute path yields [ErrPathEscapes]) and then joined with
// the prefix before being forwarded to the inner Fs.
//
// Path resolution mirrors the approach used by [osfs.Fs]: paths are
// [filepath.Clean]-ed, "." resolves to the prefix, and any non-local cleaned
// path is rejected.
type PathPrefixFs[F File] struct {
	inner  Fs[F]
	prefix string
}

// NewPathPrefixFs wraps inner so all paths are resolved relative to prefix.
// prefix is forwarded to inner verbatim and is not validated here; callers
// must supply a path meaningful to the underlying Fs.
func NewPathPrefixFs[F File](inner Fs[F], prefix string) *PathPrefixFs[F] {
	return &PathPrefixFs[F]{inner: inner, prefix: prefix}
}

func (w *PathPrefixFs[F]) resolvePath(p string) (string, error) {
	p = filepath.Clean(p)
	if p == "." {
		if w.prefix == "" {
			return ".", nil
		}
		return w.prefix, nil
	}
	if !filepath.IsLocal(p) {
		return "", ErrPathEscapes
	}
	if w.prefix == "" {
		return p, nil
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

// Name returns the configured prefix; if empty, it returns the inner Fs name.
func (w *PathPrefixFs[F]) Name() string {
	if w.prefix == "" {
		return w.inner.Name()
	}
	return w.prefix
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
