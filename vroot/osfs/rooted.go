package osfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// vendored until https://github.com/golang/go/issues/73868
// is closed
// TODO: remove when approporate
type readdirWorkAroundFile struct {
	fsys *Rooted
	name string
	*os.File
}

func wrapReaddirWorkAroundFile(fsys *Rooted, name string, f *os.File) vroot.File {
	if f == nil {
		return nil
	}
	return &readdirWorkAroundFile{fsys, name, f}
}

func (f *readdirWorkAroundFile) ReadDir(n int) ([]fs.DirEntry, error) {
	dirents, err := f.File.ReadDir(n)
	for i, dirent := range dirents {
		if dirent == nil {
			continue
		}
		dirents[i] = &readdirWorkAroundDirEntry{f.fsys, filepath.Join(f.name, dirent.Name()), dirent}
	}
	return dirents, err
}

func (f *readdirWorkAroundFile) Readdir(n int) ([]fs.FileInfo, error) {
	dirents, err := f.ReadDir(n)
	infos := make([]fs.FileInfo, 0, len(dirents))
	for _, dirent := range dirents {
		info, err := dirent.Info()
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		infos = append(infos, info)
	}
	return infos, err
}

var _ fs.DirEntry = (*readdirWorkAroundDirEntry)(nil)

type readdirWorkAroundDirEntry struct {
	fsys *Rooted
	name string
	fs.DirEntry
}

func (d *readdirWorkAroundDirEntry) Info() (fs.FileInfo, error) {
	return d.fsys.Lstat(d.name)
}

var (
	_ vroot.Rooted     = (*Rooted)(nil)
	_ vroot.ReadFileFs = (*Rooted)(nil)
)

// Rooted adapts [*os.Root] to [vroot.Rooted].
//
// Zero Rooted is invalid and must be initialied by [NewRooted].
type Rooted struct {
	root *os.Root
}

// WrapRoot returns Rooted wrapping opened [*os.Root].
func WrapRoot(root *os.Root) *Rooted {
	return &Rooted{
		root: root,
	}
}

// NewRooted opens a new Rooted on path.
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
	return wrapReaddirWorkAroundFile(r, name, f), nil
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
	return wrapReaddirWorkAroundFile(r, name, f), nil
}

func (r *Rooted) OpenFile(name string, flag int, perm fs.FileMode) (vroot.File, error) {
	f, err := r.root.OpenFile(name, flag, perm)
	if err != nil {
		return nil, swapPathEscapesErr(err)
	}
	return wrapReaddirWorkAroundFile(r, name, f), nil
}

func (r *Rooted) OpenRoot(name string) (vroot.Rooted, error) {
	root, err := r.root.OpenRoot(name)
	if err != nil {
		return nil, swapPathEscapesErr(err)
	}
	return &Rooted{root}, nil
}

func (r *Rooted) ReadLink(name string) (string, error) {
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

func (r *Rooted) ReadFile(name string) ([]byte, error) {
	return r.root.ReadFile(name)
}
