package vroot

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

var (
	SkipDir = fs.SkipDir
	SkipAll = fs.SkipAll
)

type WalkDirFunc func(path, realPath string, d fs.FileInfo, err error) error

type WalkOption struct {
	ResolveSymlink bool
}

type inode struct {
	dev   int
	inode int
}

type walkState struct {
	// maintains visited real paths.
	// either visitedPaths or visitedInodes is present
	visitedPaths map[string]struct{}
	// visitedInodes tracks visited inodes to avoid revisiting bind mounts
	// key is "device:inode"
	visitedInodes map[inode]struct{}
}

// resolveSymlinkPath resolves a symlink target to a real path relative to root
func resolveSymlinkPath(fsys Fs, linkPath string) (string, error) {
	linkPath = filepath.Clean(linkPath)
	prev := ""
	for {
		target, err := fsys.Readlink(linkPath)
		if err != nil {
			return "", err
		}

		target = filepath.Clean(target)

		linkResolved := target
		if !filepath.IsAbs(target) {
			linkResolved = filepath.Join(filepath.Dir(linkPath), target)
		}

		info, err := fsys.Lstat(linkResolved)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return linkResolved, nil
		}

		if prev == linkResolved {
			return "", fmt.Errorf("loop detected: symlinks targetting each other")
		}
		prev = linkPath
		linkPath = linkResolved
	}
}

func WalkDir(fsys Fs, root string, opt *WalkOption, fn WalkDirFunc) error {
	state := &walkState{}
	if opt == nil {
		opt = &WalkOption{}
	}

	// Use Lstat for root to avoid resolving symlinks
	info, err := fsys.Lstat(root)
	if err != nil {
		err = fn(root, root, nil, err)
	} else {
		err = walkDir(fsys, root, info, state, opt, fn)
	}
	if err == SkipDir || err == SkipAll {
		return nil
	}
	return err
}

func walkDir(
	fsys Fs,
	path string,
	info fs.FileInfo,
	state *walkState,
	opt *WalkOption,
	fn WalkDirFunc,
) error {
	realPath := path
	if opt.ResolveSymlink && info.Mode()&os.ModeSymlink != 0 {
		realPath_, err := resolveSymlinkPath(fsys, path)
		if err != nil {
			return fn(path, realPath, info, err)
		}
		realPath = realPath_
	}
	err := fn(path, realPath, info, nil)
	if err != nil || !info.IsDir() {
		if err == SkipDir && info.IsDir() {
			err = nil
		}
		return err
	}

	dirs, err := ReadDir(fsys, realPath)
	if err != nil {
		err = fn(path, realPath, nil, err)
		if err != nil {
			if err == SkipDir && info.IsDir() {
				err = nil
			}
			return err
		}
	}

	for _, dir := range dirs {
		info, err := dir.Info()
		if err != nil {
			err = fn(path, realPath, nil, err)
			if err == SkipDir && info.IsDir() {
				err = nil
			}
			return err
		}
		err = walkDir(fsys, filepath.Join(realPath, dir.Name()), info, state, opt, fn)
		if err != nil {
			if err == SkipDir {
				break
			}
			return err
		}
	}
	return nil
}
