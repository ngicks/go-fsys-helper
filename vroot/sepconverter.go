package vroot

import (
	"io/fs"
	"path/filepath"
	"strings"
	"time"
)

// toFsValidPathFs converts input path to [fs.FS] compliant form.
type toFsValidPathFs[Fsys Fs] struct {
	underlying Fsys
}

func (fsys *toFsValidPathFs[Fsys]) convertPath(p string) string {
	p = filepath.ToSlash(filepath.Clean(p))
	return strings.TrimPrefix(p, "./")
}

// Fs interface methods for toValidPathFs

func (fsys *toFsValidPathFs[Fsys]) Chmod(name string, mode fs.FileMode) error {
	return fsys.underlying.Chmod(fsys.convertPath(name), mode)
}

func (fsys *toFsValidPathFs[Fsys]) Chown(name string, uid int, gid int) error {
	return fsys.underlying.Chown(fsys.convertPath(name), uid, gid)
}

func (fsys *toFsValidPathFs[Fsys]) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return fsys.underlying.Chtimes(fsys.convertPath(name), atime, mtime)
}

func (fsys *toFsValidPathFs[Fsys]) Close() error {
	return fsys.underlying.Close()
}

func (fsys *toFsValidPathFs[Fsys]) Create(name string) (File, error) {
	return fsys.underlying.Create(fsys.convertPath(name))
}

func (fsys *toFsValidPathFs[Fsys]) Lchown(name string, uid int, gid int) error {
	return fsys.underlying.Lchown(fsys.convertPath(name), uid, gid)
}

func (fsys *toFsValidPathFs[Fsys]) Link(oldname string, newname string) error {
	return fsys.underlying.Link(fsys.convertPath(oldname), fsys.convertPath(newname))
}

func (fsys *toFsValidPathFs[Fsys]) Lstat(name string) (fs.FileInfo, error) {
	return fsys.underlying.Lstat(fsys.convertPath(name))
}

func (fsys *toFsValidPathFs[Fsys]) Mkdir(name string, perm fs.FileMode) error {
	return fsys.underlying.Mkdir(fsys.convertPath(name), perm)
}

func (fsys *toFsValidPathFs[Fsys]) MkdirAll(name string, perm fs.FileMode) error {
	return fsys.underlying.MkdirAll(fsys.convertPath(name), perm)
}

func (fsys *toFsValidPathFs[Fsys]) Name() string {
	return fsys.underlying.Name()
}

func (fsys *toFsValidPathFs[Fsys]) Open(name string) (File, error) {
	return fsys.underlying.Open(fsys.convertPath(name))
}

func (fsys *toFsValidPathFs[Fsys]) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	return fsys.underlying.OpenFile(fsys.convertPath(name), flag, perm)
}

func (fsys *toFsValidPathFs[Fsys]) OpenRoot(name string) (Rooted, error) {
	return fsys.underlying.OpenRoot(fsys.convertPath(name))
}

func (fsys *toFsValidPathFs[Fsys]) ReadLink(name string) (string, error) {
	return fsys.underlying.ReadLink(fsys.convertPath(name))
}

func (fsys *toFsValidPathFs[Fsys]) Remove(name string) error {
	return fsys.underlying.Remove(fsys.convertPath(name))
}

func (fsys *toFsValidPathFs[Fsys]) RemoveAll(name string) error {
	return fsys.underlying.RemoveAll(fsys.convertPath(name))
}

func (fsys *toFsValidPathFs[Fsys]) Rename(oldname string, newname string) error {
	return fsys.underlying.Rename(fsys.convertPath(oldname), fsys.convertPath(newname))
}

func (fsys *toFsValidPathFs[Fsys]) Stat(name string) (fs.FileInfo, error) {
	return fsys.underlying.Stat(fsys.convertPath(name))
}

func (fsys *toFsValidPathFs[Fsys]) Symlink(oldname string, newname string) error {
	return fsys.underlying.Symlink(oldname, fsys.convertPath(newname))
}

type toValidPathFsRooted[Fsys Rooted] struct {
	*toFsValidPathFs[Fsys]
}

func (fsys *toValidPathFsRooted[Fsys]) Rooted() {}

func (fsys *toValidPathFsRooted[Fsys]) OpenRoot(name string) (Rooted, error) {
	rooted, err := fsys.underlying.OpenRoot(fsys.convertPath(name))
	if err != nil {
		return nil, err
	}
	return &toValidPathFsRooted[Rooted]{&toFsValidPathFs[Rooted]{underlying: rooted}}, nil
}

type toValidPathFsUnrooted[Fsys Unrooted] struct {
	*toFsValidPathFs[Fsys]
}

func (fsys *toValidPathFsUnrooted[Fsys]) Unrooted() {}

func (fsys *toValidPathFsUnrooted[Fsys]) OpenUnrooted(name string) (Unrooted, error) {
	unrooted, err := fsys.underlying.OpenUnrooted(fsys.convertPath(name))
	if err != nil {
		return nil, err
	}
	return &toValidPathFsUnrooted[Unrooted]{&toFsValidPathFs[Unrooted]{underlying: unrooted}}, nil
}

func (fsys *toValidPathFsUnrooted[Fsys]) OpenRoot(name string) (Rooted, error) {
	rooted, err := fsys.underlying.OpenRoot(fsys.convertPath(name))
	if err != nil {
		return nil, err
	}
	return &toValidPathFsRooted[Rooted]{&toFsValidPathFs[Rooted]{underlying: rooted}}, nil
}

// toOsPathFs converts fs.ValidPath to platform-specific format.
type toOsPathFs[Fsys Fs] struct {
	underlying Fsys
}

func (fsys *toOsPathFs[Fsys]) convertPath(p string) string {
	p = filepath.FromSlash(filepath.Clean(p))
	return strings.TrimPrefix(p, "./")
}

// Fs interface methods for toOsPathFs

func (fsys *toOsPathFs[Fsys]) Chmod(name string, mode fs.FileMode) error {
	return fsys.underlying.Chmod(fsys.convertPath(name), mode)
}

func (fsys *toOsPathFs[Fsys]) Chown(name string, uid int, gid int) error {
	return fsys.underlying.Chown(fsys.convertPath(name), uid, gid)
}

func (fsys *toOsPathFs[Fsys]) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return fsys.underlying.Chtimes(fsys.convertPath(name), atime, mtime)
}

func (fsys *toOsPathFs[Fsys]) Close() error {
	return fsys.underlying.Close()
}

func (fsys *toOsPathFs[Fsys]) Create(name string) (File, error) {
	return fsys.underlying.Create(fsys.convertPath(name))
}

func (fsys *toOsPathFs[Fsys]) Lchown(name string, uid int, gid int) error {
	return fsys.underlying.Lchown(fsys.convertPath(name), uid, gid)
}

func (fsys *toOsPathFs[Fsys]) Link(oldname string, newname string) error {
	return fsys.underlying.Link(fsys.convertPath(oldname), fsys.convertPath(newname))
}

func (fsys *toOsPathFs[Fsys]) Lstat(name string) (fs.FileInfo, error) {
	return fsys.underlying.Lstat(fsys.convertPath(name))
}

func (fsys *toOsPathFs[Fsys]) Mkdir(name string, perm fs.FileMode) error {
	return fsys.underlying.Mkdir(fsys.convertPath(name), perm)
}

func (fsys *toOsPathFs[Fsys]) MkdirAll(name string, perm fs.FileMode) error {
	return fsys.underlying.MkdirAll(fsys.convertPath(name), perm)
}

func (fsys *toOsPathFs[Fsys]) Name() string {
	return fsys.underlying.Name()
}

func (fsys *toOsPathFs[Fsys]) Open(name string) (File, error) {
	return fsys.underlying.Open(fsys.convertPath(name))
}

func (fsys *toOsPathFs[Fsys]) OpenFile(name string, flag int, perm fs.FileMode) (File, error) {
	return fsys.underlying.OpenFile(fsys.convertPath(name), flag, perm)
}

func (fsys *toOsPathFs[Fsys]) OpenRoot(name string) (Rooted, error) {
	return fsys.underlying.OpenRoot(fsys.convertPath(name))
}

func (fsys *toOsPathFs[Fsys]) ReadLink(name string) (string, error) {
	return fsys.underlying.ReadLink(fsys.convertPath(name))
}

func (fsys *toOsPathFs[Fsys]) Remove(name string) error {
	return fsys.underlying.Remove(fsys.convertPath(name))
}

func (fsys *toOsPathFs[Fsys]) RemoveAll(name string) error {
	return fsys.underlying.RemoveAll(fsys.convertPath(name))
}

func (fsys *toOsPathFs[Fsys]) Rename(oldname string, newname string) error {
	return fsys.underlying.Rename(fsys.convertPath(oldname), fsys.convertPath(newname))
}

func (fsys *toOsPathFs[Fsys]) Stat(name string) (fs.FileInfo, error) {
	return fsys.underlying.Stat(fsys.convertPath(name))
}

func (fsys *toOsPathFs[Fsys]) Symlink(oldname string, newname string) error {
	return fsys.underlying.Symlink(oldname, fsys.convertPath(newname))
}

type toOsPathFsRooted[Fsys Rooted] struct {
	*toOsPathFs[Fsys]
}

func (fsys *toOsPathFsRooted[Fsys]) Rooted() {}

func (fsys *toOsPathFsRooted[Fsys]) OpenRoot(name string) (Rooted, error) {
	rooted, err := fsys.underlying.OpenRoot(fsys.convertPath(name))
	if err != nil {
		return nil, err
	}
	return &toOsPathFsRooted[Rooted]{&toOsPathFs[Rooted]{underlying: rooted}}, nil
}

type toOsPathFsUnrooted[Fsys Unrooted] struct {
	*toOsPathFs[Fsys]
}

func (fsys *toOsPathFsUnrooted[Fsys]) Unrooted() {}

func (fsys *toOsPathFsUnrooted[Fsys]) OpenUnrooted(name string) (Unrooted, error) {
	unrooted, err := fsys.underlying.OpenUnrooted(fsys.convertPath(name))
	if err != nil {
		return nil, err
	}
	return &toOsPathFsUnrooted[Unrooted]{&toOsPathFs[Unrooted]{underlying: unrooted}}, nil
}

func (fsys *toOsPathFsUnrooted[Fsys]) OpenRoot(name string) (Rooted, error) {
	rooted, err := fsys.underlying.OpenRoot(fsys.convertPath(name))
	if err != nil {
		return nil, err
	}
	return &toOsPathFsRooted[Rooted]{&toOsPathFs[Rooted]{underlying: rooted}}, nil
}

// Constructor functions

// ToFsValidPath creates a filesystem wrapper that converts platform-specific paths
// to [fs.ValidPath] compatible format (forward slash separated) before operations.
func ToFsValidPath[Fsys Fs](fsys Fsys) Fs {
	return &toFsValidPathFs[Fsys]{underlying: fsys}
}

// ToFsValidPathRooted creates a Rooted filesystem wrapper that converts platform-specific paths
// to [fs.ValidPath] compatible format before operations.
func ToFsValidPathRooted[Fsys Rooted](fsys Fsys) Rooted {
	return &toValidPathFsRooted[Fsys]{&toFsValidPathFs[Fsys]{underlying: fsys}}
}

// ToFsValidPathUnrooted creates an Unrooted filesystem wrapper that converts platform-specific paths
// to [fs.ValidPath] compatible format before operations.
func ToFsValidPathUnrooted[Fsys Unrooted](fsys Fsys) Unrooted {
	return &toValidPathFsUnrooted[Fsys]{&toFsValidPathFs[Fsys]{underlying: fsys}}
}

// ToOsPath creates a filesystem wrapper that converts fs.ValidPath format paths
// to platform-specific format (using platform separators) before operations.
func ToOsPath[Fsys Fs](fsys Fsys) Fs {
	return &toOsPathFs[Fsys]{underlying: fsys}
}

// ToOsPathRooted creates a Rooted filesystem wrapper that converts fs.ValidPath format paths
// to platform-specific format before operations.
func ToOsPathRooted[Fsys Rooted](fsys Fsys) Rooted {
	return &toOsPathFsRooted[Fsys]{&toOsPathFs[Fsys]{underlying: fsys}}
}

// ToOsPathUnrooted creates an Unrooted filesystem wrapper that converts fs.ValidPath format paths
// to platform-specific format before operations.
func ToOsPathUnrooted[Fsys Unrooted](fsys Fsys) Unrooted {
	return &toOsPathFsUnrooted[Fsys]{&toOsPathFs[Fsys]{underlying: fsys}}
}

// Verify interface compliance
var (
	_ Fs       = (*toFsValidPathFs[Fs])(nil)
	_ Rooted   = (*toValidPathFsRooted[Rooted])(nil)
	_ Unrooted = (*toValidPathFsUnrooted[Unrooted])(nil)
	_ Fs       = (*toOsPathFs[Fs])(nil)
	_ Rooted   = (*toOsPathFsRooted[Rooted])(nil)
	_ Unrooted = (*toOsPathFsUnrooted[Unrooted])(nil)
)
