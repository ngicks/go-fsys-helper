package closable

import (
	"io/fs"
	"sync/atomic"

	"github.com/spf13/afero"
)

var _ afero.File = (*Closable[afero.File])(nil)

type Closable[T afero.File] struct {
	closed atomic.Bool
	inner  afero.File
}

func NewFile[T afero.File](inner afero.File) *Closable[T] {
	return &Closable[T]{inner: inner}
}

// Close implements afero.File.
func (f *Closable[T]) Close() error {
	if f.closed.Load() {
		return fs.ErrClosed
	}
	f.closed.Store(true)
	// closing twice or more is not allowed as io.Closer spec.
	return f.inner.Close()
}

func (f *Closable[T]) errClosed(op string) error {
	if f.closed.Load() {
		return &fs.PathError{Op: op, Path: f.inner.Name(), Err: fs.ErrClosed}
	}
	return nil
}

// Name implements afero.File.
func (f *Closable[T]) Name() string {
	return f.inner.Name()
}

// Read implements afero.File.
func (f *Closable[T]) Read(p []byte) (n int, err error) {
	if err := f.errClosed("read"); err != nil {
		return 0, err
	}
	return f.inner.Read(p)
}

// ReadAt implements afero.File.
func (f *Closable[T]) ReadAt(p []byte, off int64) (n int, err error) {
	if err := f.errClosed("readat"); err != nil {
		return 0, err
	}
	return f.inner.ReadAt(p, off)
}

// Readdir implements afero.File.
func (f *Closable[T]) Readdir(count int) ([]fs.FileInfo, error) {
	if err := f.errClosed("readdir"); err != nil {
		return []fs.FileInfo{}, err
	}
	return f.inner.Readdir(count)
}

// Readdirnames implements afero.File.
func (f *Closable[T]) Readdirnames(n int) ([]string, error) {
	if err := f.errClosed("readdirnames"); err != nil {
		return []string{}, err
	}
	return f.inner.Readdirnames(n)
}

// Seek implements afero.File.
func (f *Closable[T]) Seek(offset int64, whence int) (int64, error) {
	if err := f.errClosed("seek"); err != nil {
		return 0, err
	}
	return f.inner.Seek(offset, whence)
}

// Stat implements afero.File.
func (f *Closable[T]) Stat() (fs.FileInfo, error) {
	if err := f.errClosed("stat"); err != nil {
		return nil, err
	}
	return f.inner.Stat()
}

// Sync implements afero.File.
func (f *Closable[T]) Sync() error {
	if err := f.errClosed("sync"); err != nil {
		return err
	}
	return f.inner.Sync()
}

// Truncate implements afero.File.
func (f *Closable[T]) Truncate(size int64) error {
	if err := f.errClosed("truncate"); err != nil {
		return err
	}
	return f.inner.Truncate(size)
}

// Write implements afero.File.
func (f *Closable[T]) Write(p []byte) (n int, err error) {
	if err := f.errClosed("write"); err != nil {
		return 0, err
	}
	return f.inner.Write(p)
}

// WriteAt implements afero.File.
func (f *Closable[T]) WriteAt(p []byte, off int64) (n int, err error) {
	if err := f.errClosed("writeat"); err != nil {
		return 0, err
	}
	return f.inner.WriteAt(p, off)
}

// WriteString implements afero.File.
func (f *Closable[T]) WriteString(s string) (ret int, err error) {
	if err := f.errClosed("write"); err != nil {
		return 0, err
	}
	return f.inner.WriteString(s)
}
