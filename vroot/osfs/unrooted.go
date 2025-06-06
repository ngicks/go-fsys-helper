package osfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/internal/wrapper"
)

var _ vroot.Unrooted = (*Unrooted)(nil)

// Unrooted exposes a file system under given path as [vroot.Unrooted].
// Like [*os.Root] implementation on js/wasm,
// Unrooted is vulnerable to TOCTOU(time of check, time of use) attacks.
//
// Zero value of Unrooted is invalid and must be initialized by [NewUnrooted].
type Unrooted struct {
	root string // absolute path to the root directory
}

// NewUnrooted opens a new Unrooted on path.
//
// The path must exist before NewUnrooted is called.
// It also must be a directory.
func NewUnrooted(path string) (*Unrooted, error) {
	absRoot, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	s, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !s.IsDir() {
		return nil, wrapper.PathErr("stat", absRoot, syscall.ENOTDIR)
	}

	return &Unrooted{
		root: absRoot,
	}, nil
}

func (u *Unrooted) Unrooted() {
}

func (u *Unrooted) resolvePath(path string) (string, error) {
	if u.root == "" {
		panic("calling method of zero *Unroot")
	}

	path = filepath.Clean(path)
	if path == "." {
		return u.root, nil
	}

	if !filepath.IsLocal(path) {
		return "", vroot.ErrPathEscapes
	}

	return filepath.Join(u.root, path), nil
}

func (u *Unrooted) Chmod(name string, mode fs.FileMode) error {
	path, err := u.resolvePath(name)
	if err != nil {
		return wrapper.PathErr("chmod", name, err)
	}
	return os.Chmod(path, mode)
}

func (u *Unrooted) Chown(name string, uid int, gid int) error {
	path, err := u.resolvePath(name)
	if err != nil {
		return wrapper.PathErr("chown", name, err)
	}
	return os.Chown(path, uid, gid)
}

func (u *Unrooted) Chtimes(name string, atime time.Time, mtime time.Time) error {
	path, err := u.resolvePath(name)
	if err != nil {
		return wrapper.PathErr("chtimes", name, err)
	}
	return os.Chtimes(path, atime, mtime)
}

func (u *Unrooted) Close() error {
	return nil
}

func (u *Unrooted) Create(name string) (vroot.File, error) {
	path, err := u.resolvePath(name)
	if err != nil {
		return nil, wrapper.PathErr("open", name, err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (u *Unrooted) Lchown(name string, uid int, gid int) error {
	path, err := u.resolvePath(name)
	if err != nil {
		return wrapper.PathErr("", name, err)
	}
	if u.root == path { // *os.Root resolves the given root, mimicking.
		return os.Chown(path, uid, gid)
	}
	return os.Lchown(path, uid, gid)
}

func (u *Unrooted) Link(oldname string, newname string) error {
	oldPath, err := u.resolvePath(oldname)
	if err != nil {
		return wrapper.LinkErr("link", oldname, newname, err)
	}
	newPath, err := u.resolvePath(newname)
	if err != nil {
		return wrapper.LinkErr("link", oldname, newname, err)
	}
	return os.Link(oldPath, newPath)
}

func (u *Unrooted) Lstat(name string) (fs.FileInfo, error) {
	path, err := u.resolvePath(name)
	if err != nil {
		return nil, wrapper.PathErr("lstat", name, err)
	}
	if u.root == path {
		return os.Stat(path)
	}
	return os.Lstat(path)
}

func (u *Unrooted) Mkdir(name string, perm fs.FileMode) error {
	path, err := u.resolvePath(name)
	if err != nil {
		return wrapper.PathErr("mkdir", name, err)
	}
	return os.Mkdir(path, perm)
}

func (u *Unrooted) MkdirAll(name string, perm fs.FileMode) error {
	path, err := u.resolvePath(name)
	if err != nil {
		return wrapper.PathErr("mkdir", name, err)
	}
	return os.MkdirAll(path, perm)
}

func (u *Unrooted) Name() string {
	return u.root
}

func (u *Unrooted) Open(name string) (vroot.File, error) {
	path, err := u.resolvePath(name)
	if err != nil {
		return nil, wrapper.PathErr("open", name, err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (u *Unrooted) OpenFile(name string, flag int, perm fs.FileMode) (vroot.File, error) {
	path, err := u.resolvePath(name)
	if err != nil {
		return nil, wrapper.PathErr("open", name, err)
	}
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (u *Unrooted) OpenRoot(name string) (vroot.Rooted, error) {
	path, err := u.resolvePath(name)
	if err != nil {
		return nil, wrapper.PathErr("open", name, err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	return &Rooted{root}, nil
}

func (u *Unrooted) OpenUnrooted(name string) (vroot.Unrooted, error) {
	path, err := u.resolvePath(name)
	if err != nil {
		return nil, wrapper.PathErr("open", name, err)
	}
	return NewUnrooted(path)
}

func (u *Unrooted) ReadLink(name string) (string, error) {
	path, err := u.resolvePath(name)
	if err != nil {
		return "", wrapper.PathErr("link", name, err)
	}
	if u.root == path {
		return "", wrapper.PathErr("readlink", path, syscall.EINVAL)
	}
	return os.Readlink(path)
}

func (u *Unrooted) Remove(name string) error {
	path, err := u.resolvePath(name)
	if err != nil {
		return wrapper.PathErr("", name, err)
	}
	return os.Remove(path)
}

func (u *Unrooted) RemoveAll(name string) error {
	path, err := u.resolvePath(name)
	if err != nil {
		return wrapper.PathErr("RemoveAll", name, err)
	}
	return os.RemoveAll(path)
}

func (u *Unrooted) Rename(oldname string, newname string) error {
	oldPath, err := u.resolvePath(oldname)
	if err != nil {
		return wrapper.LinkErr("rename", oldname, newname, err)
	}
	newPath, err := u.resolvePath(newname)
	if err != nil {
		return wrapper.LinkErr("rename", oldname, newname, err)
	}
	return os.Rename(oldPath, newPath)
}

func (u *Unrooted) Stat(name string) (fs.FileInfo, error) {
	path, err := u.resolvePath(name)
	if err != nil {
		return nil, wrapper.PathErr("stat", name, err)
	}
	return os.Stat(path)
}

func (u *Unrooted) Symlink(oldname string, newname string) error {
	newPath, err := u.resolvePath(newname)
	if err != nil {
		return wrapper.LinkErr("symlink", oldname, newname, err)
	}
	return os.Symlink(oldname, newPath)
}
