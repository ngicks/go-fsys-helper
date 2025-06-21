package overlayfs

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/ngicks/go-common/serr"
	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/internal/openflag"
	"github.com/ngicks/go-fsys-helper/vroot/internal/paths"
)

var _ vroot.Rooted = (*Fs)(nil)

type FsOption struct {
	CopyPolicy CopyPolicy
}

func DefaultOverlayOption() *FsOption {
	return &FsOption{
		CopyPolicy: NewCopyPolicyDotTmp("*.tmp"),
	}
}

// Fs overlays multiple layers and provides virtually concatenated view.
//
// The overlay filesystem implements a union mount where a writable top layer
// is overlaid on top of one or more read-only layers. Files and directories
// are resolved by searching from the top layer down through the lower layers.
//
// Write operations are always performed on the top layer. When modifying
// files that exist only in lower layers, they are automatically copied to
// the top layer before modification. Deleted files are tracked using whiteout
// metadata rather than actually removing files from lower layers.
type Fs struct {
	// rw shared between sub-roots.
	// TOOD: use finer lock mechanism
	rw      *sync.RWMutex
	opts    *FsOption
	top     vroot.Rooted
	topMeta MetadataStore
	layers  Layers
}

// New returns virtually concatenated view of layers as [vroot.Rooted].
//
// *Fs overlays layers left to right. A layer has higher priority than its left layers.
// On top of them, top is placed.
//
// top is used as read-write layer while other layers are assumed to be static and read-only.
// Any write operation goes to top.
//
// When modifying files that exist only in lower layers, they are automatically
// copied to the top layer using [CopyPolicy]. This includes not only Write operations
// on [vroot.File] but also Chmod, Chtimes, and other metadata modifications.
//
// When Remove or RemoveAll is called on a file that exists not only in the top layer,
// whiteout metadata is created in the top layer to make the file appear deleted.
//
// opts is allowed to be nil. In that case [DefaultOverlayOption] is used.
func New(top Layer, layers []Layer, opts *FsOption) *Fs {
	if opts == nil {
		opts = DefaultOverlayOption()
	}
	return &Fs{
		rw:      new(sync.RWMutex),
		opts:    opts,
		top:     top.fsys,
		topMeta: top.meta,
		layers:  layers,
	}
}

func (o *Fs) Rooted() {}

func (o *Fs) Name() string {
	return "overlay:" + o.top.Name()
}

func (o *Fs) Close() error {
	errs := make([]serr.PrefixErr, 1+len(o.layers))
	errs[0] = serr.PrefixErr{
		P: cmp.Or(o.top.Name(), "top layer") + ": ",
		E: o.top.Close(),
	}
	for i, layer := range o.layers {
		errs[i+1] = serr.PrefixErr{
			// one-indexed since top layer is index 0.
			P: cmp.Or(layer.Name(), fmt.Sprintf("layer %d", i+1)) + ": ",
			E: layer.Close(),
		}
	}
	return serr.GatherPrefixed(errs)
}

func doInTopOrUpperLayer[V comparable](
	top vroot.Rooted,
	topMeta MetadataStore,
	ll Layers,
	op string,
	topOp func(r vroot.Rooted, name string) (V, error),
	layerOp func(ll Layers, name string) (V, error),
	name string,
) (v V, err error) {
	name = filepath.Clean(name)

	whited, err := topMeta.QueryWhiteout(name)
	if err != nil {
		err = fsutil.WrapPathErr(op, name, err)
		return
	}
	if whited {
		err = fsutil.WrapPathErr(op, name, syscall.ENOENT)
		return
	}

	v, err = topOp(top, name)
	if err == nil || !errors.Is(err, fs.ErrNotExist) {
		return
	}

	v, err = layerOp(ll, name)
	if errors.Is(err, ErrWhitedOut) {
		err = fsutil.WrapPathErr(op, name, syscall.ENOENT)
		return
	}

	return v, err
}

func (o *Fs) lstatNoLock(name string) (fs.FileInfo, error) {
	return doInTopOrUpperLayer(
		o.top,
		o.topMeta,
		o.layers,
		"lstat",
		func(r vroot.Rooted, name string) (fs.FileInfo, error) { return r.Lstat(name) },
		Layers.Lstat,
		name,
	)
}

func (o *Fs) Lstat(name string) (fs.FileInfo, error) {
	o.rw.RLock()
	defer o.rw.RUnlock()
	return o.lstatNoLock(name)
}

func (o *Fs) readLinkNoLock(name string) (string, error) {
	return doInTopOrUpperLayer(
		o.top,
		o.topMeta,
		o.layers,
		"readlink",
		func(r vroot.Rooted, name string) (string, error) { return r.ReadLink(name) },
		Layers.ReadLink,
		name,
	)
}

func (o *Fs) ReadLink(name string) (string, error) {
	o.rw.RLock()
	defer o.rw.RUnlock()
	return o.readLinkNoLock(name)
}

type nolockOverlay struct {
	o *Fs
}

func (o *nolockOverlay) ReadLink(name string) (string, error) {
	return o.o.readLinkNoLock(name)
}

func (o *nolockOverlay) Lstat(name string) (fs.FileInfo, error) {
	return o.o.lstatNoLock(name)
}

func (o *Fs) resolvePath(name string, skipLastElement bool) (string, error) {
	return fsutil.ResolvePath(&nolockOverlay{o}, name, skipLastElement)
}

func (o *Fs) statNoLock(name string) (fs.FileInfo, error) {
	resolved, err := o.resolvePath(name, false)
	if err != nil {
		return nil, err
	}
	info, err := doInTopOrUpperLayer(
		o.top,
		o.topMeta,
		o.layers,
		"stat",
		func(r vroot.Rooted, name string) (fs.FileInfo, error) { return r.Stat(name) },
		Layers.Lstat,
		resolved,
	)
	return info, fsutil.WrapPathErr("", name, err)
}

// Stat returns file info, searching through layers and following symlinks
func (o *Fs) Stat(name string) (fs.FileInfo, error) {
	o.rw.RLock()
	defer o.rw.RUnlock()
	return o.statNoLock(name)
}

// Open opens a file for reading, searching through layers
func (o *Fs) Open(name string) (vroot.File, error) {
	return o.OpenFile(name, os.O_RDONLY, 0)
}

func (o *Fs) openMergedFileNoLock(name string, flag int, perm fs.FileMode, checkLayers bool) (vroot.File, error) {
	topFile, err := o.top.OpenFile(name, flag, perm)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	var isDir bool
	if err == nil {
		var topInfo fs.FileInfo
		topInfo, err = topFile.Stat()
		if err != nil {
			return nil, err
		}
		if !topInfo.IsDir() {
			return topFile, nil
		}
		isDir = true
	}

	var underFiles *layersFile
	if checkLayers {
		underFiles, err = o.layers.Open(name)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			_ = topFile.Close()
			return nil, err
		}
	}

	if underFiles == nil {
		if topFile == nil {
			return nil, fsutil.WrapPathErr("open", name, syscall.ENOENT)
		}
		underFiles = &layersFile{}
	} else {
		// If underFiles exists, it could also be a directory
		isDir = isDir || underFiles.isDir()
	}

	return newOverlayFile(name, isDir, topFile, underFiles), nil
}

// OpenFile opens a file with flags
func (o *Fs) openFileNoLock(name string, flag int, perm fs.FileMode) (f vroot.File, err error) {
	isWriteOp := openflag.WriteOp(flag)

	name = filepath.Clean(name)

	defer func() {
		if err != nil {
			err = fsutil.WrapPathErr("open", name, err)
		}
	}()

	// Special short cut for root dir
	if name == "." {
		return o.openMergedFileNoLock(".", flag, perm, true)
	}

	resolved, err := o.resolvePath(name, false)
	parent := filepath.Dir(resolved)
	var parentChecked bool
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if parent != "." {
			info, err := o.lstatNoLock(parent)
			if err != nil {
				return nil, err
			}
			if !info.IsDir() {
				return nil, syscall.ENOTDIR
			}
		}
		parentChecked = true
	}

	var whited bool
	if resolved != "." {
		whited, err = o.topMeta.QueryWhiteout(resolved)
		if err != nil {
			return nil, err
		}
	}

	if whited {
		if flag&os.O_CREATE == 0 {
			return nil, syscall.ENOENT
		}
		// maybe whited but file is existent; race or abnormal exit
		err = o.top.Remove(resolved)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		// Ensure parent directories exist for create operation over whiteout
		if isWriteOp && parent != "." {
			err = o.copyOnWriteNoLock(parent)
			if err != nil {
				return nil, err
			}
		}
		defer func() {
			if err == nil {
				err = o.topMeta.RemoveWhiteout(resolved)
			}
		}()
	}

	var info fs.FileInfo
	if !parentChecked && parent != "." {
		info, err = o.lstatNoLock(parent)
		if err != nil {
			return nil, err
		}

		if !info.IsDir() {
			return nil, syscall.ENOTDIR
		}
	}

	info, err = o.lstatNoLock(resolved)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err == nil && isWriteOp {
		if info.IsDir() {
			// For create operations, return ErrExist when trying to create file over directory
			if flag&os.O_CREATE != 0 {
				return nil, syscall.EEXIST
			}
			return nil, syscall.EISDIR
		}
		err = o.copyOnWriteNoLock(name)
		if err != nil {
			return nil, err
		}
	}
	return o.openMergedFileNoLock(resolved, flag, perm, !whited)
}

func (o *Fs) OpenFile(name string, flag int, perm fs.FileMode) (f vroot.File, err error) {
	isWriteOp := openflag.WriteOp(flag)

	if isWriteOp {
		o.rw.Lock()
		defer o.rw.Unlock()
	} else {
		o.rw.RLock()
		defer o.rw.RUnlock()
	}

	return o.openFileNoLock(name, flag, perm)
}

// Create creates a new file in the top layer
func (o *Fs) Create(name string) (vroot.File, error) {
	return o.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

// Remove removes a file
func (o *Fs) removeNoLock(name string) error {
	name = filepath.Clean(name)

	if name == "." || name == string(filepath.Separator) {
		return fsutil.WrapPathErr("RemoveAll", name, syscall.EINVAL)
	}

	topInfo, topErr := o.top.Lstat(name)
	if topErr != nil && !errors.Is(topErr, fs.ErrNotExist) {
		return fsutil.WrapPathErr("remove", name, topErr)
	}

	lowerInfo, lowerErr := o.layers.Lstat(name)
	if topErr != nil && lowerErr != nil {
		// At least either should be existent and accessible
		return fsutil.WrapPathErr("remove", name, topErr)
	}

	if topErr == nil && topInfo.IsDir() {
		dirents, err := vroot.ReadDir(o.top, name)
		if err != nil {
			return fsutil.WrapPathErr("remove", name, err)
		}
		if len(dirents) > 0 {
			return fsutil.WrapPathErr("remove", name, syscall.ENOTEMPTY)
		}
		// It is possible that lower directories have file and only top dir is empty.
		// Handle that case later but it's ok to remove top dir.
	}

	topIsDir := topErr == nil && topInfo.IsDir()
	lowerIsDir := lowerErr == nil && lowerInfo.IsDir()
	// shouldWhiteOut is true when,
	// 1) lower is inaccessible.
	// 2) top is dir but lower isn't dir
	// 3) top is not dir but lower is dir
	// 4) both are dir but both are also empty
	// 5) file exists only in lower layers
	shouldWhiteOut := (lowerErr != nil && !errors.Is(lowerErr, fs.ErrNotExist)) ||
		topIsDir && !lowerIsDir ||
		!topIsDir && lowerIsDir ||
		(topErr != nil && errors.Is(topErr, fs.ErrNotExist) && lowerErr == nil)

	if topIsDir && lowerIsDir {
		f, err := o.layers.Open(name)
		if err != nil {
			return fsutil.WrapPathErr("remove", name, err)
		}
		dirents, err := f.readDir(nil)
		_ = f.close()
		if err != nil {
			return fsutil.WrapPathErr("remove", name, err)
		}
		if len(dirents) == 0 {
			shouldWhiteOut = true
		}
	}

	if topErr == nil {
		err := o.top.Remove(name)
		if err != nil {
			return fsutil.WrapPathErr("remove", name, err)
		}
	}

	if shouldWhiteOut {
		err := o.topMeta.RecordWhiteout(name)
		if err != nil {
			return fsutil.WrapPathErr("remove", name, err)
		}
	}

	return nil
}

func (o *Fs) Remove(name string) error {
	o.rw.Lock()
	defer o.rw.Unlock()
	return o.removeNoLock(name)
}

func (o *Fs) removeAllNoLock(name string) error {
	name = filepath.Clean(name)

	if name == "." || name == string(filepath.Separator) {
		return fsutil.WrapPathErr("RemoveAll", name, syscall.EINVAL)
	}

	// Check if target exists
	info, err := o.lstatNoLock(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // Already doesn't exist, nothing to do
		}
		return fsutil.WrapPathErr("RemoveAll", name, err)
	}

	// If it's not a directory, just remove it
	if !info.IsDir() {
		return o.removeNoLock(name)
	}

	// If it's a directory, recursively remove contents first
	f, err := o.openFileNoLock(name, os.O_RDONLY, 0)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fsutil.WrapPathErr("RemoveAll", name, err)
	}
	defer f.Close()

	dirents, err := f.Readdir(-1)
	if err != nil {
		return fsutil.WrapPathErr("RemoveAll", name, err)
	}

	// Remove all directory contents recursively
	for _, dirent := range dirents {
		childPath := filepath.Join(name, dirent.Name())
		err := o.removeAllNoLock(childPath)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}

	// Finally remove the directory itself
	return o.removeNoLock(name)
}

// RemoveAll removes a directory tree
func (o *Fs) RemoveAll(name string) error {
	o.rw.Lock()
	defer o.rw.Unlock()
	return o.removeAllNoLock(name)
}

func (o *Fs) mkdirNoLock(name string, perm fs.FileMode) error {
	name = filepath.Clean(name)

	if name == "." {
		return fsutil.WrapPathErr("mkdir", name, syscall.EEXIST)
	}

	resolved, err := o.resolvePath(name, false)
	if err == nil && resolved == "." {
		return fsutil.WrapPathErr("mkdir", name, syscall.EEXIST)
	}
	parent := filepath.Dir(resolved)
	var parentChecked bool
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if parent != "." {
			info, err := o.lstatNoLock(parent)
			if err != nil {
				return err
			}
			if !info.IsDir() {
				return fsutil.WrapPathErr("mkdir", parent, syscall.ENOTDIR)
			}
		}
		parentChecked = true
	}

	if !parentChecked && parent != "." {
		_, err := o.statNoLock(parent)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		err = o.copyOnWriteNoLock(parent)
		if err != nil {
			return err
		}
	}

	// Check if target already exists
	info, err := o.lstatNoLock(resolved)
	if err == nil {
		if info.IsDir() {
			// Directory exists, but make sure it exists in top layer too for future operations
			_, topErr := o.top.Lstat(resolved)
			if topErr != nil && errors.Is(topErr, fs.ErrNotExist) {
				// Directory exists in overlay but not in top layer, create it in top layer
				err := o.top.Mkdir(resolved, perm)
				if err != nil && !errors.Is(err, fs.ErrExist) {
					return fsutil.WrapPathErr("mkdir", name, err)
				}
			}
			return fsutil.WrapPathErr("mkdir", name, fs.ErrExist)
		} else {
			return fsutil.WrapPathErr("mkdir", name, syscall.ENOTDIR)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fsutil.WrapPathErr("mkdir", name, err)
	}

	return o.top.Mkdir(resolved, perm)
}

// Mkdir creates a directory in the top layer
func (o *Fs) Mkdir(name string, perm fs.FileMode) error {
	o.rw.Lock()
	defer o.rw.Unlock()
	return o.mkdirNoLock(name, perm)
}

func (o *Fs) mkdirAllNoLock(name string, perm fs.FileMode) error {
	name = filepath.Clean(name)
	if name == "." {
		return nil
	}
	for path := range paths.PathFromHead(name) {
		err := o.mkdirNoLock(path, perm)
		if err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	return nil
}

func (o *Fs) MkdirAll(name string, perm fs.FileMode) error {
	o.rw.Lock()
	defer o.rw.Unlock()
	return o.mkdirAllNoLock(name, perm)
}

func (o *Fs) renameNoLock(oldname, newname string) error {
	// no path resolution: rename on link moves link itself.

	// copy to on top first
	if err := o.copyOnWriteNoLock(oldname); err != nil {
		return err
	}

	oldInfo, err := o.lstatNoLock(oldname)
	if err != nil {
		// How could this be possible...race?
		return err
	}

	newInfo, err := o.lstatNoLock(newname)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		// Some other error occurred during Lstat
		return err
	} else if err == nil {
		if (oldInfo.IsDir() && !newInfo.IsDir()) ||
			(!oldInfo.IsDir() && newInfo.IsDir()) {
			return fsutil.WrapLinkErr("rename", oldname, newname, syscall.EEXIST)
		}
		if newInfo.IsDir() {
			dirents, err := o.readLinkNoLock(newname)
			if err != nil {
				return err
			}
			if len(dirents) != 0 {
				return fsutil.WrapLinkErr("rename", oldname, newname, syscall.ENOTDIR)
			}
		}
	}

	err = o.top.Rename(oldname, newname)
	if err != nil {
		return err
	}

	if err := o.topMeta.RecordWhiteout(oldname); err != nil {
		return fsutil.WrapLinkErr("rename", oldname, newname, err)
	}
	return nil
}

func (o *Fs) Rename(oldname, newname string) error {
	o.rw.Lock()
	defer o.rw.Unlock()
	return o.renameNoLock(oldname, newname)
}

func (o *Fs) linkNoLock(oldname, newname string) error {
	_, err := o.lstatNoLock(newname)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err == nil {
		return fsutil.WrapLinkErr("link", oldname, newname, syscall.EEXIST)
	}
	if err := o.copyOnWriteNoLock(oldname); err != nil {
		return err
	}
	return o.top.Link(oldname, newname)
}

// Link creates a hard link in the top layer
func (o *Fs) Link(oldname, newname string) error {
	o.rw.Lock()
	defer o.rw.Unlock()
	return o.linkNoLock(oldname, newname)
}

func (o *Fs) symlinkNoLock(oldname, newname string) error {
	err := o.copyOnWriteNoLock(newname)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return o.top.Symlink(oldname, newname)
}

// Symlink creates a symbolic link in the top layer
func (o *Fs) Symlink(oldname, newname string) error {
	o.rw.Lock()
	defer o.rw.Unlock()
	return o.symlinkNoLock(oldname, newname)
}

// Chmod changes file permissions
func (o *Fs) chmodNoLock(name string, mode fs.FileMode) error {
	if err := o.copyOnWriteNoLock(name); err != nil {
		return err
	}
	return o.top.Chmod(name, mode)
}

func (o *Fs) Chmod(name string, mode fs.FileMode) error {
	o.rw.Lock()
	defer o.rw.Unlock()
	return o.chmodNoLock(name, mode)
}

func (o *Fs) chownNoLock(name string, uid, gid int) error {
	if err := o.copyOnWriteNoLock(name); err != nil {
		return err
	}
	return o.top.Chown(name, uid, gid)
}

func (o *Fs) Chown(name string, uid, gid int) error {
	o.rw.Lock()
	defer o.rw.Unlock()
	return o.chownNoLock(name, uid, gid)
}

func (o *Fs) Lchown(name string, uid, gid int) error {
	resolved, err := o.resolvePath(name, true)
	if err != nil {
		return err
	}
	if err := o.copyOnWriteNoLock(resolved); err != nil {
		return err
	}
	return o.top.Lchown(resolved, uid, gid)
}

func (o *Fs) chtimesNoLock(name string, atime, mtime time.Time) error {
	if err := o.copyOnWriteNoLock(name); err != nil {
		return err
	}
	return o.top.Chtimes(name, atime, mtime)
}

func (o *Fs) Chtimes(name string, atime, mtime time.Time) error {
	o.rw.Lock()
	defer o.rw.Unlock()
	return o.chtimesNoLock(name, atime, mtime)
}

// OpenRoot opens a subdirectory as a new rooted filesystem
func (o *Fs) OpenRoot(name string) (vroot.Rooted, error) {
	o.rw.Lock()
	defer o.rw.Unlock()

	name = filepath.Clean(name)

	resolved, err := o.resolvePath(name, true)
	if err != nil {
		return nil, err
	}

	info, err := o.lstatNoLock(resolved)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return nil, fsutil.WrapPathErr("open", name, syscall.ENOTDIR)
	}

	err = o.copyOnWriteNoLock(resolved)
	if err != nil {
		return nil, err
	}

	topSubRoot, err := o.top.OpenRoot(resolved)
	if err != nil {
		return nil, err
	}

	topSubMeta := SubMetadataStore(o.topMeta, resolved)

	whited, err := o.topMeta.QueryWhiteout(resolved)
	if err != nil {
		return nil, err
	}

	if whited {
		return &Fs{
			rw:      o.rw,
			opts:    o.opts,
			top:     topSubRoot,
			topMeta: topSubMeta,
			layers:  nil,
		}, nil
	}

	// Open subdirectories in other layers
	var subLayers []Layer
	for _, layer := range slices.Backward(o.layers) {
		info, err := layer.Lstat(resolved)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if errors.Is(err, ErrWhitedOut) {
				break
			}
			return nil, err
		}
		if !info.IsDir() {
			break
		}
		subRoot, err := layer.fsys.OpenRoot(resolved)
		if err != nil {
			return nil, err
		}
		subLayers = append(subLayers, Layer{
			meta: SubMetadataStore(layer.meta, resolved),
			fsys: subRoot,
		})
	}

	slices.Reverse(subLayers)

	return &Fs{
		rw:      o.rw,
		opts:    o.opts,
		top:     topSubRoot,
		topMeta: topSubMeta,
		layers:  subLayers,
	}, nil
}
