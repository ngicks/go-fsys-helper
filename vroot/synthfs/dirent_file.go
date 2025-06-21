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
	return newFileWrapper(v, f.s.name, &f.metadata), nil
}

func (f *file) readLink() (string, error) {
	return "", fsutil.WrapPathErr("readlink", f.s.name, syscall.EINVAL)
}

var _ vroot.File = (*fileWrapper)(nil)

// fileWrapper wraps a vroot.File to return platform-specific paths in Name()
// and updates the directory entry's mtime when writes occur
type fileWrapper struct {
	vroot.File
	name     string    // stored with forward slashes
	metadata *metadata // reference to directory entry's metadata
}

func newFileWrapper(f vroot.File, name string, meta *metadata) vroot.File {
	return &fileWrapper{
		File:     f,
		name:     name,
		metadata: meta,
	}
}

// Name returns the path with platform-specific separators
func (f *fileWrapper) Name() string {
	return filepath.FromSlash(f.name)
}

// syncMtimeFromView attempts to get the current mtime from the underlying view
// and update the directory entry's metadata
func (f *fileWrapper) syncMtimeFromView() {
	if f.metadata == nil {
		return
	}

	// Get current stat from the underlying file to get updated mtime
	if info, err := f.File.Stat(); err == nil {
		f.metadata.updateMtime(info.ModTime())
	}
}

// Write wraps the underlying Write and updates mtime
func (f *fileWrapper) Write(p []byte) (n int, err error) {
	n, err = f.File.Write(p)
	if err == nil && n > 0 {
		f.syncMtimeFromView()
	}
	return n, err
}

// WriteAt wraps the underlying WriteAt and updates mtime
func (f *fileWrapper) WriteAt(p []byte, off int64) (n int, err error) {
	n, err = f.File.WriteAt(p, off)
	if err == nil && n > 0 {
		f.syncMtimeFromView()
	}
	return n, err
}

// WriteString wraps the underlying WriteString and updates mtime
func (f *fileWrapper) WriteString(s string) (n int, err error) {
	n, err = f.File.WriteString(s)
	if err == nil && n > 0 {
		f.syncMtimeFromView()
	}
	return n, err
}

// Truncate wraps the underlying Truncate and updates mtime
func (f *fileWrapper) Truncate(size int64) error {
	err := f.File.Truncate(size)
	if err == nil {
		f.syncMtimeFromView()
	}
	return err
}
