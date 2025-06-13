package overlay

import (
	"errors"
	"io/fs"
	"path/filepath"
)

func copyOnWrite(name string, topLayer Layer, layers Layers, copyPolicy CopyPolicy) error {
	name = filepath.Clean(name)

	_, err := topLayer.Lstat(name)
	if err == nil {
		return nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	_, l, _, err := layers.LayerOf(name)
	if err != nil {
		return err
	}

	// Ensure parent directory exists in top layer before copying file
	dir := filepath.Dir(name)
	if dir != "." {
		// If name is already resolved, dir also cannot be symlink because it is traversing backward.
		_, err := topLayer.Lstat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			// Parent doesn't exist, copy it using copy policy
			err = copyOnWrite(dir, topLayer, layers, copyPolicy)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}

	return copyPolicy.CopyTo(l, topLayer.fsys, name)
}
