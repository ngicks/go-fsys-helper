package osfs

import (
	"errors"
	"io/fs"
	"os"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot"
)

var _ vroot.Root[*os.File, *Root] = (*Root)(nil)

// Root wraps [*os.Root] and translates the unexported "path escapes from parent" error
// returned by *os.Root into [vroot.ErrPathEscapes] so callers can use [errors.Is].
type Root struct {
	*os.Root
}

func NewRoot(name string) (*Root, error) {
	r, err := os.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	return &Root{Root: r}, nil
}

func (r *Root) IsRoot() {}

// translateEscape rewrites the leaf error of err to [vroot.ErrPathEscapes] when it matches
// the message *os.Root uses for path escape errors. Other errors are returned unchanged.
func translateEscape(err error) error {
	if err == nil {
		return nil
	}
	// *os.Root wraps its sentinel in a *fs.PathError or *os.LinkError. errors.Is on
	// the unexported sentinel can't match, so we compare the leaf error message instead.
	leaf := err
	for {
		next := errors.Unwrap(leaf)
		if next == nil {
			break
		}
		leaf = next
	}
	// Brittle: the literal must match the unexported sentinel in std `os` (`errPathEscapes`
	// in os/root.go). Re-check this on every Go release; if std changes the wording, the
	// acceptance tests for [vroot.Root] will fail and this line must be updated.
	if leaf.Error() != "path escapes from parent" {
		return err
	}
	// Rebuild the outermost error type with vroot.ErrPathEscapes as its inner cause.
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return &fs.PathError{Op: pathErr.Op, Path: pathErr.Path, Err: vroot.ErrPathEscapes}
	}
	if linkErr, ok := errors.AsType[*os.LinkError](err); ok {
		return &os.LinkError{Op: linkErr.Op, Old: linkErr.Old, New: linkErr.New, Err: vroot.ErrPathEscapes}
	}
	return vroot.ErrPathEscapes
}

func (r *Root) Chmod(name string, mode fs.FileMode) error {
	return translateEscape(r.Root.Chmod(name, mode))
}

func (r *Root) Chown(name string, uid int, gid int) error {
	return translateEscape(r.Root.Chown(name, uid, gid))
}

func (r *Root) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return translateEscape(r.Root.Chtimes(name, atime, mtime))
}

func (r *Root) Create(name string) (*os.File, error) {
	f, err := r.Root.Create(name)
	return f, translateEscape(err)
}

func (r *Root) Lchown(name string, uid int, gid int) error {
	return translateEscape(r.Root.Lchown(name, uid, gid))
}

func (r *Root) Link(oldname string, newname string) error {
	return translateEscape(r.Root.Link(oldname, newname))
}

func (r *Root) Lstat(name string) (fs.FileInfo, error) {
	info, err := r.Root.Lstat(name)
	return info, translateEscape(err)
}

func (r *Root) Mkdir(name string, perm fs.FileMode) error {
	return translateEscape(r.Root.Mkdir(name, perm))
}

func (r *Root) MkdirAll(name string, perm fs.FileMode) error {
	return translateEscape(r.Root.MkdirAll(name, perm))
}

func (r *Root) Open(name string) (*os.File, error) {
	f, err := r.Root.Open(name)
	return f, translateEscape(err)
}

func (r *Root) OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	f, err := r.Root.OpenFile(name, flag, perm)
	return f, translateEscape(err)
}

func (r *Root) ReadLink(name string) (string, error) {
	target, err := r.Root.Readlink(name)
	return target, translateEscape(err)
}

func (r *Root) Remove(name string) error {
	return translateEscape(r.Root.Remove(name))
}

func (r *Root) RemoveAll(name string) error {
	return translateEscape(r.Root.RemoveAll(name))
}

func (r *Root) Rename(oldname string, newname string) error {
	return translateEscape(r.Root.Rename(oldname, newname))
}

func (r *Root) Stat(name string) (fs.FileInfo, error) {
	info, err := r.Root.Stat(name)
	return info, translateEscape(err)
}

func (r *Root) Symlink(oldname string, newname string) error {
	return translateEscape(r.Root.Symlink(oldname, newname))
}

func (r *Root) OpenRoot(name string) (*Root, error) {
	rr, err := r.Root.OpenRoot(name)
	if err != nil {
		return nil, translateEscape(err)
	}
	return &Root{rr}, nil
}
