package vroot

import (
	"io/fs"
	"os"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot"
)

var _ vroot.Rooted = (*Rooted)(nil)

// Rooted provides secure file system access using os.Rooted.
// It prevents both path traversal and symlink escape attacks.
type Rooted struct {
	root *os.Root
}

// NewRooted creates a new Rooted instance using os.Root for the given directory.
// This provides the highest level of security by preventing both path traversal
// and symlink escapes.
func NewRooted(path string) (*Rooted, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}

	return &Rooted{
		root: root,
	}, nil
}

func (r *Rooted) Rooted() {
}

func swapPathEscapesErr(err error) error {
	switch x := err.(type) {
	case nil:
	case *fs.PathError:
		if x.Err != nil && x.Err.Error() == vroot.ErrPathEscapes.Error() {
			x.Err = vroot.ErrPathEscapes
		}
	case *os.LinkError:
		if x.Err != nil && x.Err.Error() == vroot.ErrPathEscapes.Error() {
			x.Err = vroot.ErrPathEscapes
		}
	}
	return err
}

func (r *Rooted) Chmod(name string, mode fs.FileMode) error {
	return swapPathEscapesErr(r.root.Chmod(name, mode))
}

func (r *Rooted) Chown(name string, uid int, gid int) error {
	return swapPathEscapesErr(r.root.Chown(name, uid, gid))
}

func (r *Rooted) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return swapPathEscapesErr(r.root.Chtimes(name, atime, mtime))
}

func (r *Rooted) Close() error {
	return r.root.Close()
}

func (r *Rooted) Create(name string) (vroot.File, error) {
	f, err := r.root.Create(name)
	if err != nil {
		return nil, swapPathEscapesErr(err)
	}
	return f, nil
}

func (r *Rooted) Lchown(name string, uid int, gid int) error {
	return swapPathEscapesErr(r.root.Lchown(name, uid, gid))
}

func (r *Rooted) Link(oldname string, newname string) error {
	return swapPathEscapesErr(r.root.Link(oldname, newname))
}

func (r *Rooted) Lstat(name string) (fs.FileInfo, error) {
	s, err := r.root.Lstat(name)
	if err != nil {
		return nil, swapPathEscapesErr(err)
	}
	return s, nil
}

func (r *Rooted) Mkdir(name string, perm fs.FileMode) error {
	return swapPathEscapesErr(r.root.Mkdir(name, perm))
}

func (r *Rooted) MkdirAll(name string, perm fs.FileMode) error {
	return swapPathEscapesErr(r.root.MkdirAll(name, perm))
}

func (r *Rooted) Name() string {
	return r.root.Name()
}

func (r *Rooted) Open(name string) (vroot.File, error) {
	f, err := r.root.Open(name)
	if err != nil {
		return nil, swapPathEscapesErr(err)
	}
	return f, nil
}

func (r *Rooted) OpenFile(name string, flag int, perm fs.FileMode) (vroot.File, error) {
	f, err := r.root.OpenFile(name, flag, perm)
	if err != nil {
		return nil, swapPathEscapesErr(err)
	}
	return f, nil
}

func (r *Rooted) OpenRoot(name string) (vroot.Rooted, error) {
	root, err := r.root.OpenRoot(name)
	if err != nil {
		return nil, swapPathEscapesErr(err)
	}
	return &Rooted{root}, nil
}

func (r *Rooted) Readlink(name string) (string, error) {
	s, err := r.root.Readlink(name)
	if err != nil {
		return "", swapPathEscapesErr(err)
	}
	return s, nil
}

func (r *Rooted) Remove(name string) error {
	return swapPathEscapesErr(r.root.Remove(name))
}

func (r *Rooted) RemoveAll(name string) error {
	return swapPathEscapesErr(r.root.RemoveAll(name))
}

func (r *Rooted) Rename(oldname string, newname string) error {
	return swapPathEscapesErr(r.root.Rename(oldname, newname))
}

func (r *Rooted) Stat(name string) (fs.FileInfo, error) {
	st, err := r.root.Stat(name)
	if err != nil {
		return nil, swapPathEscapesErr(err)
	}
	return st, nil
}

func (r *Rooted) Symlink(oldname string, newname string) error {
	return swapPathEscapesErr(r.root.Symlink(oldname, newname))
}
