package synthfs

import (
	"container/list"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
	"github.com/ngicks/go-fsys-helper/vroot"
)

var _ direntry = (*dir)(nil)

type dir struct {
	metadata
	parent *dir
	// ordered and direntMap hold same objects.
	// To refer them by name, use direntMap,
	// to refer them by insertion order or something, use ordered.
	//
	// ordered is needed to prevent Readdir from returning randomly ordered result.
	ordered *list.List
	files   map[string]*list.Element
}

// Methods chmod, chown, chtimes, rename, stat, and owner are inherited from metadata

func (d *dir) open(flag int) (openDirentry, error) {
	return &openDir{path: d.s.name, dir: d}, nil
}

func (d *dir) readLink() (string, error) {
	return "", fsutil.WrapPathErr("readlink", "", syscall.EINVAL)
}

func (d *dir) findDirent(name string, skipLastComponent bool, fsys *fsys) (direntry, error) {
	originalName := name
	name = filepath.Clean(name)
	if name == "." {
		return d, nil
	}

	if !filepath.IsLocal(name) {
		return nil, fsutil.WrapPathErr("open", originalName, fsutil.ErrPathEscapes)
	}

	name = filepath.ToSlash(name)

	const maxSymlinkResolution = 40
	currentResolved := 0

	currentDir := d
	var currentDirent direntry = d
	rest := name

	currentDir.mu.RLock()

	if !currentDir.s.isSearchable() {
		currentDir.mu.RUnlock()
		return nil, fsutil.WrapPathErr("open", name, syscall.EACCES)
	}

	swapDir := func(nextDir *dir) error {
		if nextDir != nil {
			nextDir.mu.RLock()
			if !nextDir.s.isSearchable() {
				nextDir.mu.RUnlock()
				return fsutil.WrapPathErr("open", name, syscall.EACCES)
			}
		}
		currentDir.mu.RUnlock()
		currentDir = nextDir
		return nil
	}
	defer swapDir(nil)

	for rest != "" && rest != "." {
		var component string
		component, rest, _ = strings.Cut(rest, "/")
		isLastComponent := len(rest) == 0

		switch component {
		case "", ".":
			continue
		case "..":
			// Check if we're trying to escape from the filesystem root
			if currentDir == fsys.root && fsys.isRooted {
				// Rooted filesystems never allow escaping the root
				return nil, fsutil.WrapPathErr("open", originalName, fsutil.ErrPathEscapes)
			}

			// Safety check for nil parent
			if currentDir.parent == nil {
				return nil, fsutil.WrapPathErr("open", originalName, fs.ErrNotExist)
			}

			if err := swapDir(currentDir.parent); err != nil {
				return nil, err
			}

			continue
		}

		elem, ok := currentDir.files[component]
		if !ok {
			return nil, fsutil.WrapPathErr("open", name, fs.ErrNotExist)
		}

		entry := elem.Value.(direntry)
		currentDirent = entry

		if !isLastComponent || !skipLastComponent {
			// Need to potentially resolve symlinks
			if symlink, ok := entry.(*symlink); ok {
				if currentResolved >= maxSymlinkResolution {
					return nil, fsutil.WrapPathErr("open", name, errdef.ELOOP)
				}
				currentResolved++

				target, _ := symlink.readLink()

				// Check for absolute symlinks - they always escape our filesystem root
				if fsys.isRooted && strings.HasPrefix(target, "/") {
					return nil, fsutil.WrapPathErr("open", name, fsutil.ErrPathEscapes)
				}

				// Relative symlink - prepend to remaining path
				if rest != "" {
					rest = target + "/" + rest
				} else {
					rest = target
				}
				continue
			}
		}

		if !isLastComponent {
			// More path components to traverse
			// Must be a directory to continue
			nextDir, ok := entry.(*dir)
			if !ok {
				return nil, fsutil.WrapPathErr("open", name, syscall.ENOTDIR)
			}

			if err := swapDir(nextDir); err != nil {
				return nil, err
			}
		} else {
			return entry, nil
		}
	}

	return currentDirent, nil
}

func (d *dir) addEntry(name string, entry direntry) {
	if d.files == nil {
		d.files = make(map[string]*list.Element)
	}
	if d.ordered == nil {
		d.ordered = list.New()
	}

	if elem, ok := d.files[name]; ok {
		d.ordered.Remove(elem)
	}

	elem := d.ordered.PushBack(entry)
	d.files[name] = elem
}

func (d *dir) removeEntry(name string) error {
	elem, ok := d.files[name]
	if !ok {
		return fsutil.WrapPathErr("remove", name, fs.ErrNotExist)
	}
	d.ordered.Remove(elem)
	delete(d.files, name)
	if f, ok := elem.Value.(*file); ok {
		_ = f.view.Close()
	}
	return nil
}

func (d *dir) listNames() []string {
	names := make([]string, 0, d.ordered.Len())
	for elem := d.ordered.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(direntry)
		info, _ := entry.stat()
		if info != nil {
			names = append(names, info.Name())
		}
	}
	return names
}

func (d *dir) listFileInfo() []fs.FileInfo {
	infos := make([]fs.FileInfo, 0, d.ordered.Len())
	for elem := d.ordered.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(direntry)
		info, err := entry.stat()
		if err == nil && info != nil {
			infos = append(infos, info)
		}
	}
	return infos
}

var (
	_ vroot.File     = (*openDir)(nil)
	_ fs.File        = (*openDir)(nil)
	_ fs.ReadDirFile = (*openDir)(nil)
)

type openDir struct {
	mu     sync.Mutex
	closed bool

	cursor int

	dir  *dir
	path string
}

func (d *openDir) checkClosed(op string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return fsutil.WrapPathErr(op, d.path, fs.ErrClosed)
	}
	return nil
}

func (d *openDir) Name() string {
	return filepath.FromSlash(d.path)
}

func (d *openDir) Stat() (fs.FileInfo, error) {
	if err := d.checkClosed("stat"); err != nil {
		return nil, err
	}
	return d.dir.stat()
}

func (d *openDir) Read([]byte) (int, error) {
	if err := d.checkClosed("read"); err != nil {
		return 0, err
	}
	return 0, fsutil.WrapPathErr("read", d.path, syscall.EISDIR)
}

func (d *openDir) ReadAt(p []byte, off int64) (n int, err error) {
	if err := d.checkClosed("readat"); err != nil {
		return 0, err
	}
	return 0, fsutil.WrapPathErr("read", d.path, syscall.EISDIR)
}

func (d *openDir) Seek(offset int64, whence int) (int64, error) {
	if err := d.checkClosed("seek"); err != nil {
		return 0, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	// mimicking *os.File behavior
	switch whence {
	default:
		return 0, fsutil.WrapPathErr("seek", d.path, fmt.Errorf("unknown whence %d: %w", whence, fs.ErrInvalid))
	case io.SeekStart:
		if offset < 0 {
			return 0, fsutil.WrapPathErr("seek", d.path, fmt.Errorf("negative offset %d: %w", offset, fs.ErrInvalid))
		}
		d.cursor = 0
	case io.SeekCurrent:
		if offset != 0 {
			return 0, fsutil.WrapPathErr("seek", d.path, fs.ErrInvalid)
		}
	case io.SeekEnd:
		if offset > 0 {
			return 0, fsutil.WrapPathErr("seek", d.path, fmt.Errorf("positive offset %d: %w", offset, fs.ErrInvalid))
		}
		d.cursor = d.dir.ordered.Len()
	}
	return 0, nil
}

func (d *openDir) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	// double close is fine
	d.closed = true
	return nil
}

func (d *openDir) ReadDir(n int) ([]fs.DirEntry, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, fsutil.WrapPathErr("readdir", d.path, fs.ErrClosed)
	}

	if d.cursor >= d.dir.ordered.Len() {
		if n <= 0 {
			return nil, nil
		}
		return nil, io.EOF
	}

	if n <= 0 {
		n = d.dir.ordered.Len() - d.cursor
	}

	entries := make([]fs.DirEntry, 0, n)
	elem := d.dir.ordered.Front()
	// Skip to cursor position
	for i := 0; i < d.cursor && elem != nil; i++ {
		elem = elem.Next()
	}

	for i := 0; i < n && elem != nil; i++ {
		entry := elem.Value.(direntry)
		info, err := entry.stat()
		if err != nil {
			return nil, err
		}
		entries = append(entries, fs.FileInfoToDirEntry(info))
		elem = elem.Next()
		d.cursor++
	}

	return entries, nil
}

func (d *openDir) Readdir(n int) ([]fs.FileInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, fsutil.WrapPathErr("readdir", d.path, fs.ErrClosed)
	}

	if d.cursor >= d.dir.ordered.Len() {
		if n <= 0 {
			return nil, nil
		}
		return nil, io.EOF
	}

	if n <= 0 {
		n = d.dir.ordered.Len() - d.cursor
	}

	infos := make([]fs.FileInfo, 0, n)
	elem := d.dir.ordered.Front()
	// Skip to cursor position
	for i := 0; i < d.cursor && elem != nil; i++ {
		elem = elem.Next()
	}

	for i := 0; i < n && elem != nil; i++ {
		entry := elem.Value.(direntry)
		info, err := entry.stat()
		if err != nil {
			return nil, err
		}
		infos = append(infos, info)
		elem = elem.Next()
		d.cursor++
	}

	return infos, nil
}

func (d *openDir) Readdirnames(n int) (names []string, err error) {
	infos, err := d.Readdir(n)
	if err != nil {
		return nil, err
	}
	names = make([]string, len(infos))
	for i, info := range infos {
		names[i] = info.Name()
	}
	return names, nil
}

// vroot.File interface methods

func (d *openDir) Chmod(mode fs.FileMode) error {
	if err := d.checkClosed("chmod"); err != nil {
		return err
	}
	d.dir.s.mode = (d.dir.s.mode &^ fs.ModePerm) | (mode & fs.ModePerm)
	return nil
}

func (d *openDir) Chown(uid, gid int) error {
	if err := d.checkClosed("chown"); err != nil {
		return err
	}
	d.dir.uid = uid
	d.dir.gid = gid
	return nil
}

func (d *openDir) Fd() uintptr {
	return ^(uintptr(0))
}

func (d *openDir) Sync() error {
	return d.checkClosed("sync")
}

func (d *openDir) Truncate(size int64) error {
	if err := d.checkClosed("truncate"); err != nil {
		return err
	}
	return fsutil.WrapPathErr("truncate", d.path, syscall.EISDIR)
}

func (d *openDir) Write(b []byte) (n int, err error) {
	if err := d.checkClosed("write"); err != nil {
		return 0, err
	}
	return 0, fsutil.WrapPathErr("write", d.path, syscall.EISDIR)
}

func (d *openDir) WriteAt(b []byte, off int64) (n int, err error) {
	if err := d.checkClosed("writeat"); err != nil {
		return 0, err
	}
	return 0, fsutil.WrapPathErr("write", d.path, syscall.EISDIR)
}

func (d *openDir) WriteString(s string) (n int, err error) {
	if err := d.checkClosed("writestring"); err != nil {
		return 0, err
	}
	return 0, fsutil.WrapPathErr("write", d.path, syscall.EISDIR)
}
