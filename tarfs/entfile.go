package tarfs

import (
	"io"
	"io/fs"
	"sync"
	"syscall"
)

type file struct {
	h *Section
}

func (f *file) header() *Section {
	return f.h
}

func (f *file) open(r io.ReaderAt, path string) openDirentry {
	return &openFile{
		r:        makeReader(r, f.h),
		path:     path,
		fileInfo: f.header(),
		file:     f,
	}
}

func (f *file) readLink() (string, error) {
	return "", pathErr("readlink", "", syscall.EINVAL)
}

var _ fs.File = (*openFile)(nil)

type openFile struct {
	mu       sync.Mutex
	closed   bool
	readMu   sync.Mutex // guards Read, Seek, not for ReadAt since the method does not need a guard.
	r        seekReadReaderAt
	path     string
	file     *file
	fileInfo *Section // maybe overriten by hardlink
}

func (f *openFile) checkClosed(op string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return pathErr(op, f.path, fs.ErrClosed)
	}
	return nil
}

func (f *openFile) Name() string {
	return f.path
}

func (f *openFile) Stat() (fs.FileInfo, error) {
	if err := f.checkClosed("stat"); err != nil {
		return nil, err
	}
	return f.fileInfo.Header().FileInfo(), nil
}

func (f *openFile) Read(p []byte) (n int, err error) {
	if err := f.checkClosed("read"); err != nil {
		return 0, err
	}
	f.readMu.Lock()
	defer f.readMu.Unlock()
	n, err = f.r.Read(p)
	if err != nil {
		err = pathErr("read", f.path, err)
	}
	return
}

func (f *openFile) ReadAt(p []byte, off int64) (n int, err error) {
	if err := f.checkClosed("readat"); err != nil {
		return 0, err
	}
	n, err = f.r.ReadAt(p, off)
	if err != nil {
		err = pathErr("read", f.path, err)
	}
	return
}

func (f *openFile) Seek(offset int64, whence int) (int64, error) {
	if err := f.checkClosed("seek"); err != nil {
		return 0, err
	}
	f.readMu.Lock()
	defer f.readMu.Unlock()
	n, err := f.r.Seek(offset, whence)
	if err != nil {
		err = pathErr("seek", f.path, err)
	}
	return n, err
}

func (f *openFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// double close is fine for this.
	f.closed = true
	return nil
}
