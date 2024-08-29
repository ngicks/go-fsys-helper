package fsutil

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/afero"
)

func CleanDir(fsys afero.Fs, path string) error {
	err := cleanDir(fsys, path)
	if err != nil {
		return fmt.Errorf("fsutil.CleanDir: %w", err)
	}
	return nil
}

func cleanDir(fsys afero.Fs, path string) error {
	dir, err := fsys.Open(path)
	if err != nil {
		return err
	}
	dirents, err := dir.Readdir(-1)
	if err != nil {
		return err
	}
	for _, dirent := range dirents {
		if err := fsys.RemoveAll(filepath.Join(path, dirent.Name())); err != nil {
			return err
		}
	}
	return nil
}
