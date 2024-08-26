package vmesh

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	pathpkg "path"
	"strings"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/aferofs/clock"
	"github.com/spf13/afero"
)

var _ afero.Fs = (*Fs)(nil)

// Fs constructs a virtual mesh that links other filesystem's content or in-memory content,
// exposing them as afero.Fs.
//
// To make up virtual structure of filesystem, call [Fs.AddFile] or
// pass non-nil [FileViewAllocator] to [New] and call [Fs.Create] or [Fs.OpenFile] with os.O_CREATE flag.
//
// Fs behaves as an in-memory filesystem if created with [MemFileAllocator].
type Fs struct {
	umask     fs.FileMode
	clock     clock.WallClock
	root      *dirent
	allocator FileViewAllocator
}

func newFsys(umask fs.FileMode, allocator FileViewAllocator, opt ...FsOption) *Fs {
	fsys := &Fs{
		umask:     umask.Perm(),
		clock:     clock.RealWallClock(),
		allocator: allocator,
	}
	for _, o := range opt {
		o.apply(fsys)
	}
	fsys.root = &dirent{name: ".", dir: newDirData(fs.ModePerm, fsys.clock.Now())}
	return fsys
}

func New(umask fs.FileMode, allocator FileViewAllocator, opt ...FsOption) *Fs {
	return newFsys(umask, allocator, opt...)
}

func NewNoAlloc(umask fs.FileMode, opt ...FsOption) *Fs {
	return newFsys(umask, nil, opt...)
}

func (fsys *Fs) maskPerm(perm fs.FileMode) fs.FileMode {
	return perm.Perm() &^ fsys.umask
}

func wrapErr(op string, path string, e error) error {
	if e == nil {
		return nil
	}
	if e == io.EOF {
		// don't wrap the sentinel value.
		return e
	}
	if pErr, ok := e.(*fs.PathError); ok {
		if pErr.Path == "" {
			pErr.Path = path
		}
		if pErr.Op == "" {
			pErr.Op = op
		}
		return pErr
	}
	return &fs.PathError{Op: op, Path: path, Err: e}
}

func validatePath(path string) error {
	if !fs.ValidPath(path) {
		return fmt.Errorf("%w: fs.ValidPath returned false", fs.ErrInvalid)
	}
	if len(pathpkg.Base(path)) > 255 {
		// For many unix filesystem implementations,
		// name is limited no more than 255 bytes.
		// Many implementations also uses UTF-8 for path encoding.
		return syscall.ENAMETOOLONG
	}
	return nil
}

func (fsys *Fs) findParent(path string) (*dirent, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	if path == "." {
		return fsys.root, nil
	}

	// chmod may change root dir's perm
	// In that case you can't do anything.
	if err := fsys.root.IsSearchableDir(); err != nil {
		return nil, err
	}

	path, _ = strings.CutSuffix(path, "/")

	var (
		parentName string
		parent     *dirent = fsys.root
	)
	for {
		// for foo/bar/baz, check for foo, goto next round
		parentName, path, _ = strings.Cut(path, "/")
		if len(path) == 0 {
			return parent, nil
		}
		parent, _ = parent.lookup(parentName)
		if err := parent.IsSearchableDir(); err != nil {
			return nil, err
		}
	}
}

func (fsys *Fs) find(path string) (*dirent, error) {
	parent, err := fsys.findParent(path)
	if err != nil {
		return nil, err
	}
	basename := pathpkg.Base(path)
	if basename == "." {
		return parent, nil
	}
	ent, ok := parent.lookup(basename)
	if !ok {
		return nil, syscall.ENOENT
	}
	return ent, nil
}

func (fsys *Fs) findWritableDir(path string) (*dirent, error) {
	dirent, err := fsys.find(path)
	if err != nil {
		return nil, err
	}
	if err := dirent.IsDirErr(); err != nil {
		return nil, err
	}
	if !dirent.hasPerm(0o2) {
		return nil, syscall.EACCES
	}
	return dirent, nil
}

func permErr(dirent *dirent, perm int) error {
	if !dirent.hasPerm(perm) {
		return syscall.EACCES
	}
	return nil
}

func lookupParent(parent *dirent, name string) (*dirent, error) {
	basename := pathpkg.Base(name)
	if basename == "." {
		return nil, syscall.EPERM
	}
	dirent, ok := parent.lookup(basename)
	if !ok {
		return nil, syscall.ENOENT
	}
	return dirent, nil
}

func (fsys *Fs) Chmod(name string, mode fs.FileMode) error {
	ent, err := fsys.find(name)
	if err != nil {
		return wrapErr("chmod", name, err)
	}
	// Fs owns all files inside. So no permission checked.
	ent.chmod(mode)
	return nil
}

func (fsys *Fs) Chown(name string, uid int, gid int) error {
	// uid and gid are currently not used.
	// May eventually be exposed to implement https://pkg.go.dev/archive/tar#FileInfoNames
	ent, err := fsys.find(name)
	if err != nil {
		return wrapErr("chown", name, err)
	}
	ent.chown(uid, gid)
	return nil
}

func (fsys *Fs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	ent, err := fsys.find(name)
	if err != nil {
		return wrapErr("chtimes", name, err)
	}
	ent.chtimes(atime, mtime)
	return nil
}

func (fsys *Fs) Create(name string) (afero.File, error) {
	return fsys.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
}

func (fsys *Fs) Mkdir(name string, perm fs.FileMode) error {
	err := fsys.mkdir(name, perm)
	return wrapErr("mkdir", name, err)
}

func (fys *Fs) mkdir(name string, perm fs.FileMode) error {
	parent, err := fys.findWritableDir(path.Dir(name))
	if err != nil {
		return err
	}

	basename := pathpkg.Base(name)
	if basename == "." {
		// The root dir always exists, cannot be removed.
		return syscall.EEXIST
	}

	_, ok := parent.lookup(basename)
	if ok {
		return syscall.EEXIST
	}

	if !parent.hasPerm(0o1) {
		return syscall.EACCES
	}
	if !parent.hasPerm(0o2) {
		return syscall.EPERM
	}

	parent.addDirent(newDirDirent(basename, fys.maskPerm(perm), fys.clock.Now()))

	return nil
}

func (fsys *Fs) MkdirAll(path string, perm fs.FileMode) error {
	err := fsys.mkdirAll(path, perm)
	return wrapErr("mkdir", path, err)
}

func (fsys *Fs) mkdirAll(path string, perm fs.FileMode) error {
	if err := validatePath(path); err != nil {
		return err
	}

	if path == "." {
		// already created.
		return nil
	}

	if err := fsys.root.IsSearchableDir(); err != nil {
		return err
	}

	org := path
	path, _ = strings.CutSuffix(path, "/")

	var (
		top            string
		currentPathIdx int
		parent         *dirent = fsys.root
		child          *dirent
		ok             bool
	)
	for len(path) > 0 {
		// for foo/bar/baz, check for foo, goto next round
		top, path, _ = strings.Cut(path, "/")

		if currentPathIdx > 0 {
			currentPathIdx++ // for /
		}
		currentPathIdx += len(top)

		child, ok = parent.lookup(top)
		if !ok {
			child = newDirDirent(top, fsys.maskPerm(perm), fsys.clock.Now())
			parent.addDirent(child)
		}

		if err := child.IsSearchableDir(); err != nil {
			return wrapErr("mkdir", org[:currentPathIdx], err)
		}
		parent = child
	}

	return nil
}

func (fsys *Fs) Name() string {
	return "github.com/ngicks/go-fsys-helper/aferofs/vmesh.Fs"
}

func (fsys *Fs) Open(name string) (afero.File, error) {
	return fsys.OpenFile(name, os.O_RDONLY, 0)
}

func (fsys *Fs) OpenFile(path string, flag int, perm fs.FileMode) (afero.File, error) {
	f, err := fsys.openFile(path, flag, perm)
	return f, wrapErr("open", path, err)
}

func (fsys *Fs) openFile(name string, flag int, perm fs.FileMode) (afero.File, error) {
	parent, err := fsys.findParent(name)
	if err != nil {
		return nil, err
	}

	basename := pathpkg.Base(name)
	if basename == "." {
		return newOpenHandle(name, flag, fsys.root)
	}

	ent, ok := parent.lookup(basename)
	if ok {
		if flag&os.O_EXCL != 0 {
			return nil, syscall.EEXIST
		}
		targetPerm := flagPerm(flag)
		if !ent.hasPerm(targetPerm) {
			return nil, syscall.EACCES
		}
		if ent.IsDir() &&
			(flagWritable(flag) || flag&os.O_TRUNC != 0) {
			return nil, syscall.EISDIR
		}
		if flag&os.O_TRUNC != 0 {
			// https://man7.org/linux/man-pages/man2/open.2.html#VERSIONS
			//
			// > The (undefined) effect of O_RDONLY | O_TRUNC varies among
			// > implementations.  On many systems the file is actually truncated.
			err = ent.file.Truncate(0)
			if err != nil {
				return nil, err
			}
		}
		return newOpenHandle(name, flag, ent)
	}

	if flag&os.O_CREATE == 0 {
		return nil, syscall.ENOENT
	}

	if !parent.hasPerm(0o3) {
		return nil, syscall.EACCES
	}

	if fsys.allocator == nil {
		return nil, syscall.EROFS
	}

	data := fsys.allocator.Allocate(name, perm)
	f, err := newFileDirent(data, name)
	if err != nil {
		return nil, err
	}
	opened, err := newOpenHandle(name, flag, f)
	if err != nil {
		return nil, err
	}
	parent.addDirent(f)
	return opened, nil
}

func (fsys *Fs) Remove(name string) error {
	parent, err := fsys.findParent(name)
	if err != nil {
		return wrapErr("remove", name, err)
	}
	err = removeFromParent(parent, name)
	if err != nil {
		return wrapErr("remove", name, err)
	}
	return nil
}

func removeFromParent(parent *dirent, name string) error {
	basename := pathpkg.Base(name)
	if basename == "." {
		return syscall.EPERM
	}
	if !parent.hasPerm(0o3) {
		return syscall.EACCES
	}
	ent, ok := parent.lookup(basename)
	if !ok {
		return syscall.ENOENT
	}
	if ent.IsDir() && ent.len() > 0 {
		return syscall.ENOTEMPTY
	}
	err := ent.notifyClose()
	parent.removeName(basename)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrClosedWithError, err)
	}
	return nil
}

func (fsys *Fs) RemoveAll(name string) error {
	err := fsys.removeAll(name)
	return wrapErr("remove", name, err)
}

func (fsys *Fs) removeAll(name string) error {
	if err := validatePath(name); err != nil {
		return err
	}

	if name == "." {
		// can't remove
		return syscall.EACCES
	}

	err := fsys.Remove(name)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	parent, err := fsys.findParent(name)
	if err != nil {
		return err
	}

	errorPath, err := removeAllFrom(parent, pathpkg.Base(name))
	if err != nil {
		return &fs.PathError{Op: "remove", Path: errorPath, Err: err}
	}
	return nil
}

func removeAllFrom(parent *dirent, name string) (path string, err error) {
	err = removeFromParent(parent, name)
	if err == nil || errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrClosedWithError) {
		return "", nil
	}
	if !errors.Is(err, syscall.ENOTEMPTY) {
		return name, err
	}
	dir, _ := parent.lookup(name)
	for _, name := range dir.dir.ListName() {
		path, err = removeAllFrom(dir, name)
		if err != nil {
			return name + "/" + path, err
		}
	}
	return "", nil
}

func (fsys *Fs) Rename(oldname string, newname string) error {
	err := fsys.rename(oldname, newname)
	return wrapErr("rename", oldname, err)
}

func (fsys *Fs) rename(oldname string, newname string) error {
	if err := validatePath(oldname); err != nil {
		return &fs.PathError{Path: oldname, Err: err}
	}
	if err := validatePath(newname); err != nil {
		return &fs.PathError{Path: newname, Err: err}
	}

	if oldname == newname {
		// > https://man7.org/linux/man-pages/man2/rename.2.html
		// >
		// > If oldpath and newpath are existing hard links referring to the
		// > same file, then rename() does nothing, and returns a success
		// > status.
		return nil
	}

	if oldname == "." || newname == "." {
		// The root cannot be moved or overwritten.
		//
		// > https://man7.org/linux/man-pages/man2/rename.2.html
		// >
		// >  EBUSY  The rename fails because oldpath or newpath is a directory
		// >     that is in use by some process (perhaps as current working
		// >     directory, or as root directory,
		return syscall.EBUSY
	}

	// as per https://man7.org/linux/man-pages/man2/rename.2.html#ERRORS

	// Paths that pass the fs.ValidPath never have trailing slash.
	if strings.HasPrefix(newname+"/", oldname+"/") {
		return &fs.PathError{Path: oldname, Err: syscall.EINVAL}
	}

	findDirent := func(name string) (parent *dirent, target *dirent, err error) {
		parent, err = fsys.findParent(name)
		if err != nil {
			return
		}
		if err = permErr(parent, 2); err != nil {
			return
		}
		target, err = lookupParent(parent, name)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				err = nil
			}
		}
		return
	}

	oldParent, oldTarget, err := findDirent(oldname)
	if err != nil {
		return err
	}
	if oldTarget == nil {
		return syscall.ENOENT
	}

	newParent, newTarget, err := findDirent(newname)
	if err != nil {
		return wrapErr("rename", newname, err)
	}

	if oldTarget.IsDir() && !oldTarget.hasPerm(2) {
		return syscall.EACCES
	}

	if newTarget != nil {
		if oldTarget.IsFile() && newTarget.IsDir() {
			return &fs.PathError{Path: oldname, Err: syscall.EISDIR}
		}
		if oldTarget.IsDir() && newTarget.IsFile() {
			return &fs.PathError{Path: oldname, Err: syscall.ENOTDIR}
		}
		if oldTarget.IsDir() && newTarget.IsDir() && newTarget.len() > 0 {
			return &fs.PathError{Path: newname, Err: syscall.ENOTEMPTY}
		}
	}

	oldParent.removeDirent(oldTarget)
	replaced := newParent.addDirent(oldTarget)
	if replaced != nil {
		replaced.notifyClose()
	}
	oldTarget.notifyRename(newname)

	return nil

}

func (fsys *Fs) Stat(name string) (fs.FileInfo, error) {
	ent, err := fsys.find(name)
	if err != nil {
		return nil, wrapErr("stat", name, err)
	}
	return ent.stat()
}
