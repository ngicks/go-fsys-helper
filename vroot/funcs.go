package vroot

import (
	"io"
	"io/fs"
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
// Otherwise returns invalid value(0xffffffff).
func Fd(f any) uintptr {
	if fdFile, ok := f.(fdFile); ok {
		return fdFile.Fd()
	}
	return 0xffffffff
}
