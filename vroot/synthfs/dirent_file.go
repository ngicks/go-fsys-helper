package synthfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

var _ direntry = (*file)(nil)

type file struct {
	metadata
	view FileView
}

func (f *file) stat() (fs.FileInfo, error) {
	// For file, we need to get size from the view
	v, err := f.view.Open(os.O_RDONLY)
	if err != nil {
		return nil, fsutil.WrapPathErr("stat", f.s.name, err)
	}
	defer v.Close()
	info, err := v.Stat()
	if err != nil {
		return nil, fsutil.WrapPathErr("stat", f.s.name, err)
	}

	f.mu.RLock()
	defer f.mu.RUnlock()
	s := f.s
	s.size = info.Size()
	// Don't override modTime - it should be managed by chtimes
	return s, nil
}

func (f *file) open(flag int) (openDirentry, error) {
	v, err := f.view.Open(flag)
	if err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	return newFileWrapper(v, f.s.name), nil
}

func (f *file) readLink() (string, error) {
	return "", fsutil.WrapPathErr("readlink", f.s.name, syscall.EINVAL)
}

var _ vroot.File = (*fileWrapper)(nil)

// fileWrapper wraps a vroot.File to return platform-specific paths in Name()
type fileWrapper struct {
	vroot.File
	name string // stored with forward slashes
}

func newFileWrapper(f vroot.File, name string) vroot.File {
	return &fileWrapper{
		File: f,
		name: name,
	}
}

// Name returns the path with platform-specific separators
func (f *fileWrapper) Name() string {
	return filepath.FromSlash(f.name)
}
