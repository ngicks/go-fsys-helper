package synthfs

import (
	"container/list"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/clock"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
	"github.com/ngicks/go-fsys-helper/vroot/internal/paths"
)

// Option configures a synthetic filesystem.
type Option struct {
	// Clock is used for timestamps. If nil, clock.RealWallClock() will be used.
	Clock clock.WallClock
	// Umask is the file mode mask applied to new files and directories.
	// If nil, defaults to 0o022. Set to pointer to zero to disable umask.
	Umask *fs.FileMode
}

// applyDefaults returns an Option with default values filled in.
func (o Option) applyDefaults() Option {
	if o.Clock == nil {
		o.Clock = clock.RealWallClock()
	}
	if o.Umask == nil {
		defaultUmask := fs.FileMode(0o022)
		o.Umask = &defaultUmask
	}
	return o
}

var (
	_ vroot.Fs       = (*fsys)(nil)
	_ vroot.Rooted   = (*Rooted)(nil)
	_ vroot.Unrooted = (*Unrooted)(nil)
)

type fsys struct {
	umask     fs.FileMode
	clock     clock.WallClock
	root      *dir
	allocator FileViewAllocator
	isRooted  bool
	name      string
}

// Rooted constructs a synthetic filesystem that combines file-like views from different data sources,
// to synthesize them into an imitation filesystem.
//
// Fs accepts different data sources or backing storage as a virtual file.
// [Rooted.AddFile] adds file-like view backed by arbitrary implementations into Fs.
// Or passing [FileViewAllocator] to [NewRooed] will allocate a new file-like view using it when [*Rooted.Create] or
// [*Rooted.OpenFile] with os.O_CREATE flag is called.
//
// Rooted tries its best to mimic ext4 on the linux.
// So it may have difference when running on windows.
type Rooted struct {
	*fsys
}

// Rooted interface marker
func (r *Rooted) Rooted() {}

// AddFile adds a file view to the filesystem at the specified path.
// If the parent directory doesn't exist, it will be created with the specified permissions.
// The file will be added with the specified permissions in its metadata.
func (r *Rooted) AddFile(name string, view FileView, dirPerm, filePerm fs.FileMode) error {
	return r.fsys.AddFile(name, view, dirPerm, filePerm)
}

// AddFs adds all files from the given vroot.Fs to the filesystem under the specified root directory.
// If the root directory doesn't exist, it will be created with the specified permissions.
// All directories and files will be created with the specified permissions.
func (r *Rooted) AddFs(root string, vrootFs vroot.Fs, perm fs.FileMode) error {
	return r.fsys.AddFs(root, vrootFs, perm)
}

// Unrooted is unrooted version of [*Rooted]
type Unrooted struct {
	*fsys
}

// Unrooted interface marker
func (u *Unrooted) Unrooted() {}

// AddFile adds a file view to the filesystem at the specified path.
// If the parent directory doesn't exist, it will be created with the specified permissions.
// The file will be added with the specified permissions in its metadata.
func (u *Unrooted) AddFile(name string, view FileView, dirPerm, filePerm fs.FileMode) error {
	return u.fsys.AddFile(name, view, dirPerm, filePerm)
}

// AddFs adds all files from the given vroot.Fs to the filesystem under the specified root directory.
// If the root directory doesn't exist, it will be created with the specified permissions.
// All directories and files will be created with the specified permissions.
func (u *Unrooted) AddFs(root string, vrootFs vroot.Fs, perm fs.FileMode) error {
	return u.fsys.AddFs(root, vrootFs, perm)
}

// OpenUnrooted opens an unrooted view of the filesystem
func (u *Unrooted) OpenUnrooted(name string) (vroot.Unrooted, error) {
	name = toSlash(name)
	// Find the named directory
	dirent, err := u.root.findDirent(name, false, u.fsys)
	if err != nil {
		return nil, err
	}

	// Must be a directory
	dir, ok := dirent.(*dir)
	if !ok {
		return nil, fsutil.WrapPathErr("open", name, syscall.ENOTDIR)
	}

	return &Unrooted{
		fsys: &fsys{
			umask:     u.umask,
			clock:     u.clock,
			root:      dir,
			allocator: u.allocator,
			isRooted:  false,
			name:      path.Join(u.name, name),
		},
	}, nil
}

// newFs creates a new synthetic filesystem.
func newFs(name string, allocator FileViewAllocator, opt Option, isRooted bool) *fsys {
	opt = opt.applyDefaults()

	root := &dir{
		metadata: metadata{
			s: stat{
				mode:    fs.ModeDir | 0o755,
				modTime: opt.Clock.Now(),
				name:    ".",
			},
		},
		ordered: list.New(),
		files:   make(map[string]*list.Element),
	}

	return &fsys{
		umask:     *opt.Umask,
		clock:     opt.Clock,
		root:      root,
		allocator: allocator,
		isRooted:  isRooted,
		name:      name,
	}
}

// NewRooted creates a new rooted synthetic filesystem.
func NewRooted(name string, allocator FileViewAllocator, opt Option) *Rooted {
	return &Rooted{
		fsys: newFs(name, allocator, opt, true),
	}
}

// NewUnrooted creates a new unrooted synthetic filesystem.
func NewUnrooted(name string, allocator FileViewAllocator, opt Option) *Unrooted {
	return &Unrooted{
		fsys: newFs(name, allocator, opt, false),
	}
}

func (f *fsys) Name() string {
	return f.name
}

func (f *fsys) Close() error {
	// No-op for synthetic filesystem
	return nil
}

// toSlash converts a path to use forward slashes for internal use
func toSlash(name string) string {
	return filepath.ToSlash(name)
}

func (f *fsys) Chmod(name string, mode fs.FileMode) error {
	name = toSlash(name)
	dirent, err := f.root.findDirent(name, false, f)
	if err != nil {
		return err
	}
	dirent.chmod(mode)
	return nil
}

func (f *fsys) Chown(name string, uid int, gid int) error {
	name = toSlash(name)
	dirent, err := f.root.findDirent(name, false, f)
	if err != nil {
		return err
	}
	dirent.chown(uid, gid)
	return nil
}

func (f *fsys) Chtimes(name string, atime time.Time, mtime time.Time) error {
	name = toSlash(name)
	dirent, err := f.root.findDirent(name, false, f)
	if err != nil {
		return err
	}
	return dirent.chtimes(atime, mtime)
}

func (f *fsys) Create(name string) (vroot.File, error) {
	return f.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

func (f *fsys) Lchown(name string, uid int, gid int) error {
	name = toSlash(name)
	dirent, err := f.root.findDirent(name, true, f)
	if err != nil {
		return err
	}
	dirent.chown(uid, gid)
	return nil
}

func (f *fsys) Link(oldname string, newname string) error {
	oldname = toSlash(oldname)
	newname = toSlash(newname)

	// Find the source entry
	source, err := f.root.findDirent(oldname, false, f)
	if err != nil {
		return err
	}

	// Reject link to directory
	if _, ok := source.(*dir); ok {
		return fsutil.WrapPathErr("link", oldname, syscall.EPERM)
	}

	// Only files can be hard linked
	sourceFile, ok := source.(*file)
	if !ok {
		return fsutil.WrapPathErr("link", oldname, fs.ErrInvalid)
	}

	// Find parent directory for new name
	cleanName := path.Clean(newname)
	dirPath, base := path.Split(cleanName)
	if dirPath == "" {
		dirPath = "."
	}

	parentDirent, err := f.root.findDirent(strings.TrimSuffix(dirPath, "/"), false, f)
	if err != nil {
		return err
	}

	parentDir, ok := parentDirent.(*dir)
	if !ok {
		return fsutil.WrapPathErr("link", newname, syscall.ENOTDIR)
	}

	// Check if parent is writable
	if !parentDir.s.isWritable() {
		return fsutil.WrapPathErr("link", newname, syscall.EACCES)
	}

	// Create a new file entry that shares the same view
	newFile := &file{
		metadata: metadata{
			s: stat{
				mode:    sourceFile.s.mode,
				modTime: sourceFile.s.modTime,
				name:    cleanName,
				size:    sourceFile.s.size,
			},
			uid: sourceFile.uid,
			gid: sourceFile.gid,
		},
		view: sourceFile.view, // Share the same view
	}

	parentDir.mu.Lock()
	defer parentDir.mu.Unlock()

	// Check if already exists
	if _, ok := parentDir.files[base]; ok {
		return fsutil.WrapPathErr("link", newname, fs.ErrExist)
	}

	parentDir.addEntry(base, newFile)

	return nil
}

func (f *fsys) Lstat(name string) (fs.FileInfo, error) {
	name = toSlash(name)
	dirent, err := f.root.findDirent(name, true, f)
	if err != nil {
		return nil, err
	}
	return dirent.stat()
}

func (f *fsys) Mkdir(name string, perm fs.FileMode) error {
	name = toSlash(name)
	cleanName := path.Clean(name)
	dirPath, base := path.Split(cleanName)
	if dirPath == "" {
		dirPath = "."
	}

	parentDirent, err := f.root.findDirent(strings.TrimSuffix(dirPath, "/"), false, f)
	if err != nil {
		return err
	}

	parentDir, ok := parentDirent.(*dir)
	if !ok {
		return fsutil.WrapPathErr("mkdir", name, syscall.ENOTDIR)
	}

	// Check if writable
	if !parentDir.s.isWritable() {
		return fsutil.WrapPathErr("mkdir", name, syscall.EACCES)
	}

	newDir := &dir{
		metadata: metadata{
			s: stat{
				mode:    fs.ModeDir | (perm &^ f.umask),
				modTime: f.clock.Now(),
				name:    cleanName,
			},
		},
		parent:  parentDir,
		ordered: list.New(),
		files:   make(map[string]*list.Element),
	}

	parentDir.mu.Lock()
	defer parentDir.mu.Unlock()

	// Check if already exists
	if _, ok := parentDir.files[base]; ok {
		return fsutil.WrapPathErr("mkdir", name, fs.ErrExist)
	}

	parentDir.addEntry(base, newDir)
	return nil
}

func (f *fsys) MkdirAll(name string, perm fs.FileMode) error {
	name = filepath.Clean(name)
	if name == "." {
		return nil
	}
	for path := range paths.PathFromHead(name) {
		err := f.Mkdir(path, perm)
		if err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	return nil
}

func (f *fsys) Open(name string) (vroot.File, error) {
	return f.OpenFile(name, os.O_RDONLY, 0)
}

func (f *fsys) OpenFile(name string, flag int, perm fs.FileMode) (vroot.File, error) {
	name = toSlash(name)

	// Handle create flag
	if flag&os.O_CREATE != 0 {
		cleanName := path.Clean(name)
		dirPath, base := path.Split(cleanName)
		if dirPath == "" {
			dirPath = "."
		}

		parentDirent, err := f.root.findDirent(strings.TrimSuffix(dirPath, "/"), false, f)
		if err != nil {
			return nil, err
		}

		parentDir, ok := parentDirent.(*dir)
		if !ok {
			return nil, fsutil.WrapPathErr("open", name, syscall.ENOTDIR)
		}

		// Check if parent is writable
		if !parentDir.s.isWritable() {
			return nil, fsutil.WrapPathErr("open", name, syscall.EACCES)
		}

		parentDir.mu.Lock()
		_, exists := parentDir.files[base]

		if exists && flag&os.O_EXCL != 0 {
			parentDir.mu.Unlock()
			return nil, fsutil.WrapPathErr("open", name, fs.ErrExist)
		}

		if !exists {
			// Create new file
			view := f.allocator.Allocate(cleanName, perm&^f.umask)
			newFile := &file{
				metadata: metadata{
					s: stat{
						mode:    perm &^ f.umask,
						modTime: f.clock.Now(),
						name:    cleanName,
					},
				},
				view: view,
			}
			parentDir.addEntry(base, newFile)
			parentDir.mu.Unlock()

			return newFile.open(flag)
		}
		parentDir.mu.Unlock()
	}

	// Open existing entry
	dirent, err := f.root.findDirent(name, false, f)
	if err != nil {
		return nil, err
	}

	// Check permissions
	info, _ := dirent.stat()
	if openflag.Writable(flag) {
		if !info.(stat).isWritable() {
			return nil, fsutil.WrapPathErr("open", name, syscall.EACCES)
		}
	} else {
		if !info.(stat).isReadable() {
			return nil, fsutil.WrapPathErr("open", name, syscall.EACCES)
		}
	}

	return dirent.open(flag)
}

func (f *fsys) OpenRoot(name string) (vroot.Rooted, error) {
	name = toSlash(name)
	// Find the named directory
	dirent, err := f.root.findDirent(name, false, f)
	if err != nil {
		return nil, err
	}

	// Must be a directory
	dir, ok := dirent.(*dir)
	if !ok {
		return nil, fsutil.WrapPathErr("openroot", name, syscall.ENOTDIR)
	}

	return &Rooted{
		fsys: &fsys{
			umask:     f.umask,
			clock:     f.clock,
			root:      dir,
			allocator: f.allocator,
			isRooted:  true,
			name:      path.Join(f.name, name),
		},
	}, nil
}

func (f *fsys) ReadLink(name string) (string, error) {
	name = toSlash(name)
	dirent, err := f.root.findDirent(name, true, f)
	if err != nil {
		return "", err
	}
	s, err := dirent.readLink()
	s = filepath.FromSlash(s)
	return s, err
}

func (f *fsys) Remove(name string) error {
	name = toSlash(name)
	cleanName := path.Clean(name)
	dirPath, base := path.Split(cleanName)
	if dirPath == "" {
		dirPath = "."
	}

	parentDirent, err := f.root.findDirent(strings.TrimSuffix(dirPath, "/"), false, f)
	if err != nil {
		return err
	}

	parentDir, ok := parentDirent.(*dir)
	if !ok {
		return fsutil.WrapPathErr("remove", name, syscall.ENOTDIR)
	}

	// Check if parent is writable
	if !parentDir.s.isWritable() {
		return fsutil.WrapPathErr("remove", name, syscall.EACCES)
	}

	parentDir.mu.Lock()
	defer parentDir.mu.Unlock()

	elem, ok := parentDir.files[base]
	if !ok {
		return fsutil.WrapPathErr("remove", name, fs.ErrNotExist)
	}

	// Check if it's a non-empty directory
	if dir, ok := elem.Value.(*dir); ok {
		if dir.ordered.Len() > 0 {
			return fsutil.WrapPathErr("remove", name, syscall.ENOTEMPTY)
		}
	}

	return parentDir.removeEntry(base)
}

func (f *fsys) RemoveAll(name string) error {
	name = toSlash(name)

	// First, check if the path exists
	dirent, err := f.root.findDirent(name, true, f)
	if err != nil {
		// If it doesn't exist, RemoveAll succeeds (like os.RemoveAll)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	// If it's a directory, we need to remove all contents first
	if dir, ok := dirent.(*dir); ok {
		// Get a list of all entries to remove
		dir.mu.RLock()
		var entries []string
		for elem := dir.ordered.Front(); elem != nil; elem = elem.Next() {
			entry := elem.Value.(direntry)
			info, _ := entry.stat()
			if info != nil {
				entries = append(entries, info.Name())
			}
		}
		dir.mu.RUnlock()

		// Remove all entries recursively
		for _, entry := range entries {
			entryPath := path.Join(name, entry)
			if err := f.RemoveAll(entryPath); err != nil {
				return err
			}
		}
	}

	// Now remove the item itself
	return f.Remove(name)
}

func (f *fsys) Rename(oldname string, newname string) error {
	oldname = toSlash(oldname)
	newname = toSlash(newname)

	// Find the source entry
	oldClean := path.Clean(oldname)
	oldDir, oldBase := path.Split(oldClean)
	if oldDir == "" {
		oldDir = "."
	}

	oldParentDirent, err := f.root.findDirent(strings.TrimSuffix(oldDir, "/"), false, f)
	if err != nil {
		return err
	}

	oldParentDir, ok := oldParentDirent.(*dir)
	if !ok {
		return fsutil.WrapPathErr("rename", oldname, syscall.ENOTDIR)
	}

	// Check if old parent is writable
	if !oldParentDir.s.isWritable() {
		return fsutil.WrapPathErr("rename", oldname, syscall.EACCES)
	}

	// Find the destination parent
	newClean := path.Clean(newname)
	newDir, newBase := path.Split(newClean)
	if newDir == "" {
		newDir = "."
	}

	newParentDirent, err := f.root.findDirent(strings.TrimSuffix(newDir, "/"), false, f)
	if err != nil {
		return err
	}

	newParentDir, ok := newParentDirent.(*dir)
	if !ok {
		return fsutil.WrapPathErr("rename", newname, syscall.ENOTDIR)
	}

	// Check if new parent is writable
	if !newParentDir.s.isWritable() {
		return fsutil.WrapPathErr("rename", newname, syscall.EACCES)
	}

	// Lock both directories (always in the same order to avoid deadlock)
	if oldParentDir == newParentDir {
		oldParentDir.mu.Lock()
		defer oldParentDir.mu.Unlock()
	} else if oldParentDir.s.name < newParentDir.s.name {
		oldParentDir.mu.Lock()
		defer oldParentDir.mu.Unlock()
		newParentDir.mu.Lock()
		defer newParentDir.mu.Unlock()
	} else {
		newParentDir.mu.Lock()
		defer newParentDir.mu.Unlock()
		oldParentDir.mu.Lock()
		defer oldParentDir.mu.Unlock()
	}

	// Get the source entry
	elem, ok := oldParentDir.files[oldBase]
	if !ok {
		return fsutil.WrapPathErr("rename", oldname, fs.ErrNotExist)
	}

	entry := elem.Value.(direntry)

	// Check if destination exists
	if _, exists := newParentDir.files[newBase]; exists {
		return fsutil.WrapPathErr("rename", newname, fs.ErrExist)
	}

	// Remove from old location
	oldParentDir.ordered.Remove(elem)
	delete(oldParentDir.files, oldBase)

	// Update the entry's name
	entry.rename(newClean)

	// If it's a directory, update parent pointer
	if dir, ok := entry.(*dir); ok {
		dir.parent = newParentDir
	}

	// Add to new location
	newParentDir.addEntry(newBase, entry)

	return nil
}

func (f *fsys) Stat(name string) (fs.FileInfo, error) {
	name = toSlash(name)
	dirent, err := f.root.findDirent(name, false, f)
	if err != nil {
		return nil, err
	}
	return dirent.stat()
}

func (f *fsys) Symlink(oldname string, newname string) error {
	oldname = toSlash(oldname)
	newname = toSlash(newname)
	cleanName := path.Clean(newname)
	dirPath, base := path.Split(cleanName)
	if dirPath == "" {
		dirPath = "."
	}

	parentDirent, err := f.root.findDirent(strings.TrimSuffix(dirPath, "/"), false, f)
	if err != nil {
		return err
	}

	parentDir, ok := parentDirent.(*dir)
	if !ok {
		return fsutil.WrapPathErr("symlink", newname, syscall.ENOTDIR)
	}

	// Check if parent is writable
	if !parentDir.s.isWritable() {
		return fsutil.WrapPathErr("symlink", newname, syscall.EACCES)
	}

	newSymlink := &symlink{
		metadata: metadata{
			s: stat{
				mode:    fs.ModeSymlink | 0o777,
				modTime: f.clock.Now(),
				name:    cleanName,
			},
		},
		target: oldname,
	}

	parentDir.mu.Lock()
	defer parentDir.mu.Unlock()

	// Check if already exists
	if _, ok := parentDir.files[base]; ok {
		return fsutil.WrapPathErr("symlink", newname, fs.ErrExist)
	}

	parentDir.addEntry(base, newSymlink)
	return nil
}
