package vroot

import (
	"io"
	"io/fs"
	"path/filepath"
)

func ReadDir(fsys Fs, name string) ([]fs.DirEntry, error) {
	f, err := fsys.Open(filepath.FromSlash(name))
	if err != nil {
		return []fs.DirEntry{}, err
	}
	defer f.Close()
	return f.ReadDir(-1)
}

func ReadFile(fsys Fs, name string) ([]byte, error) {
	f, err := fsys.Open(filepath.FromSlash(name))
	if err != nil {
		return []byte{}, err
	}
	defer f.Close()
	// Do we need to implement an optimization that is what os.ReadFile does?
	return io.ReadAll(f)
}
