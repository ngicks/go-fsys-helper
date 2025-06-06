package vroot

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/ngicks/go-fsys-helper/vroot/internal/wrapper"
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

// resolveSymlink resolves a symlink using the provided base path for relative targets
func resolveSymlink(fsys Fs, linkPath, realParentPath string) (string, error) {
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
			// Resolve relative to the real parent directory where this symlink exists
			realLinkDir := realParentPath
			if realParentPath != filepath.Dir(linkPath) {
				// parentRealPath is the real path of the parent directory
				realLinkDir = realParentPath
			} else {
				realLinkDir = filepath.Dir(linkPath)
			}
			linkResolved = filepath.Join(realLinkDir, target)
			linkResolved = filepath.Clean(linkResolved)
		}

		info, err := fsys.Lstat(linkResolved)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return linkResolved, nil
		}

		if prev == linkResolved {
			return "", wrapper.PathErr("statat", linkPath, syscall.ELOOP)
		}

		prev = linkPath
		linkPath = linkResolved
		realParentPath = filepath.Dir(linkResolved)
	}
}

func WalkDir(fsys Fs, root string, opt *WalkOption, fn WalkDirFunc) error {
	state := &walkState{
		visitedPaths: make(map[string]struct{}),
	}
	if opt == nil {
		opt = &WalkOption{}
	}

	// Use Lstat for root to avoid resolving symlinks
	info, err := fsys.Lstat(root)
	if err != nil {
		err = fn(root, root, nil, err)
	} else {
		err = walkDir(fsys, root, root, info, state, opt, fn)
	}
	if err == SkipDir || err == SkipAll {
		return nil
	}
	return err
}

func walkDir(
	fsys Fs,
	path string,
	realPath string,
	info fs.FileInfo,
	state *walkState,
	opt *WalkOption,
	fn WalkDirFunc,
) error {
	if opt.ResolveSymlink && info.Mode()&os.ModeSymlink != 0 {
		var (
			err       error
			realPath_ string
		)
		info, err = fsys.Stat(path)
		if err == nil {
			realPath_, err = resolveSymlink(fsys, path, filepath.Dir(realPath))
		}
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

	// Check for filesystem loops by tracking visited real paths
	if info.IsDir() {
		if _, visited := state.visitedPaths[realPath]; visited {
			// Skip this directory to avoid infinite loops
			return nil
		}
		state.visitedPaths[realPath] = struct{}{}
		defer delete(state.visitedPaths, realPath)
	}

	dirs, err := ReadDir(fsys, path)
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
		childPath := filepath.Join(path, dir.Name())
		childRealPath := filepath.Join(realPath, dir.Name())

		info, err := fsys.Lstat(childRealPath)
		if err != nil {
			err = fn(childPath, childRealPath, nil, err)
			if err == SkipDir && info != nil && info.IsDir() {
				err = nil
			}
			return err
		}
		err = walkDir(fsys, childPath, childRealPath, info, state, opt, fn)
		if err != nil {
			if err == SkipDir {
				break
			}
			return err
		}
	}
	return nil
}
