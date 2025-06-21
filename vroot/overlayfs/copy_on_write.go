package overlayfs

import (
	"errors"
	"io/fs"
	"path/filepath"
	"syscall"
)

func (o *Fs) topAsLayer() *Layer {
	return &Layer{o.topMeta, o.top}
}

func (o *Fs) copyOnWriteNoLock(name string) error {
	name = filepath.Clean(name)

	_, err := o.topAsLayer().Lstat(name)
	if err == nil {
		return nil
	}

	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	_, l, _, err := o.layers.LayerOf(name)
	if err != nil {
		return err
	}

	// Ensure parent directory exists in top layer before copying file
	dir := filepath.Dir(name)
	if dir != "." {
		// If name is already resolved, dir also cannot be symlink because it is traversing backward.
		_, err := o.topAsLayer().Lstat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			info, err := o.layers.Lstat(dir)
			if err == nil && !info.IsDir() {
				return &fs.PathError{Op: "open", Path: dir, Err: syscall.ENOTDIR}
			}
			// Parent doesn't exist, copy it using copy policy
			err = o.copyOnWriteNoLock(dir)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}

	return o.opts.CopyPolicy.CopyTo(l, o.top, name)
}
