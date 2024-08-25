package aferofs

import (
	"fmt"
	"io"
	"io/fs"
	"unsafe"
)

func ValidPathErr(path string) error {
	if !fs.ValidPath(path) {
		return fmt.Errorf("%w: fs.ValidPath returned false", fs.ErrInvalid)
	}
	return nil
}

func ReadAt(f fs.File, p []byte, off int64) (n int, err error) {
	if wa, ok := f.(io.ReaderAt); ok {
		return wa.ReadAt(p, off)
	}
	return 0, fmt.Errorf("%w: readat", ErrOpNotSupported)
}

type ReaderDir interface {
	Readdir(count int) ([]fs.FileInfo, error)
}

func Readdir(f fs.File, count int) ([]fs.FileInfo, error) {
	if r, ok := f.(ReaderDir); ok {
		return r.Readdir(count)
	}
	return []fs.FileInfo{}, fmt.Errorf("%w: readdir", ErrOpNotSupported)
}

type ReaderDirnames interface {
	Readdirnames(n int) ([]string, error)
}

func Readdirnames(f fs.File, n int) ([]string, error) {
	if r, ok := f.(ReaderDirnames); ok {
		return r.Readdirnames(n)
	}
	fileInfo, err := Readdir(f, n)
	if err != nil {
		return []string{}, err
	}
	str := make([]string, len(fileInfo))
	for i, fi := range fileInfo {
		str[i] = fi.Name()
	}
	return str, nil
}

func Seek(f fs.File, offset int64, whence int) (int64, error) {
	if s, ok := f.(io.Seeker); ok {
		return s.Seek(offset, whence)
	}
	return 0, fmt.Errorf("%w: seek", ErrOpNotSupported)
}

type Syncer interface {
	Sync() error
}

func Sync(f fs.File) error {
	if s, ok := f.(Syncer); ok {
		return s.Sync()
	}
	return fmt.Errorf("%w: sync", ErrOpNotSupported)
}

type Truncator interface {
	Truncate(size int64) error
}

func Truncate(f fs.File, size int64) error {
	if t, ok := f.(Truncator); ok {
		return t.Truncate(size)
	}
	return fmt.Errorf("%w: truncate", ErrOpNotSupported)
}

func Write(f fs.File, p []byte) (n int, err error) {
	if w, ok := f.(io.Writer); ok {
		return w.Write(p)
	}
	return 0, fmt.Errorf("%w: write", ErrOpNotSupported)
}

func WriteAt(f fs.File, p []byte, off int64) (n int, err error) {
	if wa, ok := f.(io.WriterAt); ok {
		return wa.WriteAt(p, off)
	}
	return 0, fmt.Errorf("%w: writeat", ErrOpNotSupported)
}

func WriteString(f fs.File, s string) (ret int, err error) {
	if ws, ok := f.(io.StringWriter); ok {
		return ws.WriteString(s)
	}
	if w, ok := f.(io.Writer); ok {
		b := unsafe.Slice(unsafe.StringData(s), len(s))
		return w.Write(b)
	}
	return 0, fmt.Errorf("%w: write", ErrOpNotSupported)
}
