// Package osfs provides [vroot.Fs] and [vroot.Root] implementations backed
// by the real operating-system filesystem via [os.OpenRoot].
//
// Create, Open and OpenFile return [*File], on [Fs] and [Root] alike. It is a
// thin wrapper over [*os.File] that promotes every method of it and adds the one
// the stdlib type cannot carry: [vroot.Locker], through flock(2) on the platforms
// that have whole-file advisory locks and LockFileEx on windows — where there are
// none, [File] carries no Lock or Unlock at all rather than a pair that refuses.
// These methods returned *os.File before, so a caller naming the concrete type
// has to name [*File] now; one going through [vroot.File] or [vroot.Fs] sees no
// change.
package osfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

var (
	_ vroot.Fs[*File] = (*Fs)(nil)
)

// Fs exposes a file system under given path as [vroot.Fs].
// Like [*os.Root] implementation on js/wasm,
// Fs is vulnerable to TOCTOU(time of check, time of use) attacks,
// and also reopening sub fs-es may be needed after renaming their root.
//
// Zero value of Fs is invalid and must be initialized by [NewFs].
type Fs struct {
	root string // absolute path to the root directory
}

// NewFs opens a new Fs on the path.
//
// The path must exist before NewFs is called.
// It also must be a directory.
func NewFs(path string) (*Fs, error) {
	absRoot, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	s, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !s.IsDir() {
		return nil, fsutil.WrapPathErr("stat", absRoot, syscall.ENOTDIR)
	}

	return &Fs{
		root: absRoot,
	}, nil
}

func (u *Fs) resolvePath(path string) (string, error) {
	if u.root == "" {
		panic("calling method of zero *Fs")
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

func (u *Fs) Chmod(name string, mode fs.FileMode) error {
	path, err := u.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("chmod", name, err)
	}
	return os.Chmod(path, mode)
}

func (u *Fs) Chown(name string, uid int, gid int) error {
	path, err := u.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("chown", name, err)
	}
	return os.Chown(path, uid, gid)
}

func (u *Fs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	path, err := u.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("chtimes", name, err)
	}
	return os.Chtimes(path, atime, mtime)
}

func (u *Fs) Close() error {
	return nil
}

func (u *Fs) Create(name string) (*File, error) {
	path, err := u.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &File{f}, nil
}

func (u *Fs) Lchown(name string, uid int, gid int) error {
	path, err := u.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("", name, err)
	}
	if u.root == path { // *os.Root resolves the given root, mimicking.
		return os.Chown(path, uid, gid)
	}
	return os.Lchown(path, uid, gid)
}

func (u *Fs) Link(oldname string, newname string) error {
	oldPath, err := u.resolvePath(oldname)
	if err != nil {
		return fsutil.WrapLinkErr("link", oldname, newname, err)
	}
	newPath, err := u.resolvePath(newname)
	if err != nil {
		return fsutil.WrapLinkErr("link", oldname, newname, err)
	}
	return os.Link(oldPath, newPath)
}

func (u *Fs) Lstat(name string) (fs.FileInfo, error) {
	path, err := u.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("lstat", name, err)
	}
	if u.root == path {
		return os.Stat(path)
	}
	return os.Lstat(path)
}

func (u *Fs) Mkdir(name string, perm fs.FileMode) error {
	path, err := u.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("mkdir", name, err)
	}
	return os.Mkdir(path, perm)
}

func (u *Fs) MkdirAll(name string, perm fs.FileMode) error {
	path, err := u.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("mkdir", name, err)
	}
	return os.MkdirAll(path, perm)
}

func (u *Fs) Name() string {
	return u.root
}

func (u *Fs) Open(name string) (*File, error) {
	path, err := u.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &File{f}, nil
}

func (u *Fs) OpenFile(name string, flag int, perm fs.FileMode) (*File, error) {
	path, err := u.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}
	return &File{f}, nil
}

func (u *Fs) OpenRoot(name string) (*Root, error) {
	path, err := u.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	return &Root{Root: root}, nil
}

func (u *Fs) ReadLink(name string) (string, error) {
	path, err := u.resolvePath(name)
	if err != nil {
		return "", fsutil.WrapPathErr("link", name, err)
	}
	if u.root == path {
		// behave as if root is always already resolved.
		return "", fsutil.WrapPathErr("readlink", path, syscall.EINVAL)
	}
	return os.Readlink(path)
}

func (u *Fs) Remove(name string) error {
	path, err := u.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("", name, err)
	}
	return os.Remove(path)
}

func (u *Fs) RemoveAll(name string) error {
	// Reject paths whose final element is "." before cleaning, matching
	// os.RemoveAll / *os.Root.RemoveAll which return EINVAL (rmdir(2) refuses
	// to remove "."). resolvePath would otherwise filepath.Clean "tree/." down
	// to "tree" and remove it.
	if endsWithDot(name) {
		return fsutil.WrapPathErr("RemoveAll", name, syscall.EINVAL)
	}
	path, err := u.resolvePath(name)
	if err != nil {
		return fsutil.WrapPathErr("RemoveAll", name, err)
	}
	if path == u.root {
		// consistency to os.RemoveAll and *os.Root.RemoveAll
		return fsutil.WrapPathErr("RemoveAll", ".", syscall.EINVAL)
	}
	return os.RemoveAll(path)
}

// endsWithDot reports whether the final path element of name is ".". It mirrors
// the check in the standard library's os.RemoveAll (os/removeall_*.go).
func endsWithDot(name string) bool {
	if name == "." {
		return true
	}
	if len(name) >= 2 && name[len(name)-1] == '.' && os.IsPathSeparator(name[len(name)-2]) {
		return true
	}
	return false
}

func (u *Fs) Rename(oldname string, newname string) error {
	oldPath, err := u.resolvePath(oldname)
	if err != nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, err)
	}
	newPath, err := u.resolvePath(newname)
	if err != nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, err)
	}
	return os.Rename(oldPath, newPath)
}

func (u *Fs) Stat(name string) (fs.FileInfo, error) {
	path, err := u.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("stat", name, err)
	}
	return os.Stat(path)
}

func (u *Fs) Symlink(oldname string, newname string) error {
	newPath, err := u.resolvePath(newname)
	if err != nil {
		return fsutil.WrapLinkErr("symlink", oldname, newname, err)
	}
	return os.Symlink(oldname, newPath)
}

func (u *Fs) ReadFile(name string) ([]byte, error) {
	newName, err := u.resolvePath(name)
	if err != nil {
		return nil, fsutil.WrapPathErr("open", name, err)
	}
	return os.ReadFile(newName)
}
