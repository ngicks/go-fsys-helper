package vroot

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ngicks/go-fsys-helper/fsutil"
)

var (
	SkipDir = fs.SkipDir
	SkipAll = fs.SkipAll
)

type WalkDirFunc func(path, realPath string, d fs.FileInfo, err error) error

type WalkOption struct {
	ResolveSymlink bool
}

func WalkDir(fsys Fs, root string, opt *WalkOption, fn WalkDirFunc) error {
	return walkDir(fsys, root, opt, fn)
}

type walkState struct {
	// maintains visited real paths.
	// either visitedPaths or visitedInodes is present
	visitedPaths map[string]struct{}
	// visitedInodes tracks visited inodes to avoid revisiting bind mounts
	// key is "device:inode"
	visitedInodes map[fileIdent]struct{}
	// remaning number of symlink resolution allowed.
	symlinkResolveRemaining int
}

var logUniqueness = false

func (s *walkState) recordVisited(fsys Fs, virtualPath, realPath string, info fs.FileInfo) (visited bool) {
	ident, ok := fileIdentFromSys(fsys, virtualPath, realPath, info)
	if logUniqueness {
		fmt.Printf("%q: %#v\n", realPath, ident)
	}
	if ok {
		// Here it is trying to find loop by dev:inode.
		// This is suppose to break file system loop by bind mounts.
		if s.visitedInodes == nil {
			s.visitedInodes = map[fileIdent]struct{}{}
		}
		if _, visited := s.visitedInodes[ident]; visited {
			// Skip this directory to avoid infinite loops
			return true
		}
		s.visitedInodes[ident] = struct{}{}
		return false
	} else {
		if realPath == "" {
			// can't determine
			return false
		}

		if s.visitedPaths == nil {
			s.visitedPaths = map[string]struct{}{}
		}
		if _, visited := s.visitedPaths[realPath]; visited {
			// Skip this directory to avoid infinite loops
			return true
		}
		s.visitedPaths[realPath] = struct{}{}
		return false
	}
}

func (s *walkState) removeVisited(fsys Fs, virtualPath, realPath string, info fs.FileInfo) {
	ident, ok := fileIdentFromSys(fsys, virtualPath, realPath, info)
	if ok {
		delete(s.visitedInodes, ident)
	} else {
		delete(s.visitedPaths, realPath)
	}
}

func walkDir(fsys Fs, root string, opt *WalkOption, fn WalkDirFunc) error {
	state := &walkState{
		symlinkResolveRemaining: 40, // following linux's recent max
	}
	if opt == nil {
		opt = &WalkOption{}
	}

	// Use Lstat for root to avoid resolving symlinks
	info, err := fsys.Lstat(root)
	if err != nil {
		err = fn(root, root, nil, err)
	} else {
		err = walkDir_(fsys, root, root, info, state, opt, fn)
	}
	if err == SkipDir || err == SkipAll {
		return nil
	}
	return err
}

func walkDir_(
	fsys Fs,
	path string,
	realPath string,
	info fs.FileInfo,
	state *walkState,
	opt *WalkOption,
	fn WalkDirFunc,
) error {
	path = filepath.Clean(path)

	if opt.ResolveSymlink && info.Mode()&os.ModeSymlink != 0 {
		var (
			err       error
			realPath_ string
		)
		info, err = fsys.Stat(path)
		if err == nil && realPath != "" {
			var numResolved int
			realPath_, numResolved, err = fsutil.ResolveSymlink(fsys, realPath, state.symlinkResolveRemaining)
			state.symlinkResolveRemaining -= numResolved
			defer func() {
				state.symlinkResolveRemaining += numResolved
			}()
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

	if info.IsDir() {
		if visited := state.recordVisited(fsys, path, realPath, info); visited {
			// already visited; loop detected.
			return nil
		}
		defer state.removeVisited(fsys, path, realPath, info)
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
		childRealPath := ""
		if realPath != "" {
			childRealPath = filepath.Join(realPath, dir.Name())
		}
		info, err := fsys.Lstat(childPath)
		if err != nil {
			err = fn(childPath, childRealPath, nil, err)
			if err == SkipDir && info != nil && info.IsDir() {
				err = nil
			}
			return err
		}
		err = walkDir_(fsys, childPath, childRealPath, info, state, opt, fn)
		if err != nil {
			if err == SkipDir {
				break
			}
			return err
		}
	}
	return nil
}
