package vroot

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type ReadDirFs interface {
	Fs
	ReadDir(name string) ([]fs.DirEntry, error)
}

func ReadDir(fsys Fs, name string) ([]fs.DirEntry, error) {
	if readDirFsys, ok := fsys.(ReadDirFs); ok {
		return readDirFsys.ReadDir(name)
	}

	f, err := fsys.Open(filepath.FromSlash(name))
	if err != nil {
		return []fs.DirEntry{}, err
	}
	defer f.Close()
	return f.ReadDir(-1)
}

type ReadFileFs interface {
	Fs
	ReadFile(name string) ([]byte, error)
}

func ReadFile(fsys Fs, name string) ([]byte, error) {
	if readFileFsys, ok := fsys.(ReadFileFs); ok {
		return readFileFsys.ReadFile(name)
	}

	f, err := fsys.Open(filepath.FromSlash(name))
	if err != nil {
		return []byte{}, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

type fdFile interface {
	Fd() uintptr
}

// Fd returns fd of f if it implements interface{ Fd() uintptr }.
// Otherwise returns invalid value(^(uintptr(0))).
func Fd(f any) uintptr {
	if ff, ok := f.(fdFile); ok {
		return ff.Fd()
	}
	return ^(uintptr(0))
}

// WriteFile is short hand for creating file at name and writing data into it.
func WriteFile(fsys Fs, name string, data []byte, perm fs.FileMode) error {
	f, err := fsys.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err1 := f.Close(); err1 != nil && err == nil {
		err = err1
	}
	return err
}
