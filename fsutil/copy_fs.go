package fsutil

import (
	"errors"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"

	"github.com/ngicks/go-fsys-helper/fsutil/internal/bufpool"
	pathspkg "github.com/ngicks/go-fsys-helper/fsutil/internal/paths"
)

type copyFsFile interface {
	WriteFile
	CloseFile
	NameFile
	SyncFile
}

type copyFsFsys[File copyFsFile] interface {
	OpenFileFs[File]
	MkdirFs
	ChmodFs
}

// CopyFsOption configures filesystem copy operations.
type CopyFsOption[Fsys copyFsFsys[File], File copyFsFile] struct {
	// ChmodMask is used to mask file permissions during chmod operations.
	// If zero, [fs.ModePerm] is used as the default mask.
	// For os-backed filesystems, consider setting this to [ChmodMask]
	ChmodMask fs.FileMode
}

// maskPerm returns the permission masked with ChmodMask.
// If ChmodMask is zero, returns perm & fs.ModePerm.
func (opt CopyFsOption[Fsys, File]) maskPerm(perm fs.FileMode) fs.FileMode {
	mask := opt.ChmodMask
	if mask == 0 {
		mask = fs.ModePerm
	}
	return perm & mask
}

// CopyAll performs recursive copy from src filesystem to dst filesystem under the specified root path.
func (opt CopyFsOption[Fsys, File]) CopyAll(dst Fsys, src fs.FS, root string) error {
	srcLstat, hasLstat := src.(interface {
		Lstat(name string) (fs.FileInfo, error)
	})
	return fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		var (
			info    fs.FileInfo
			statErr error
		)
		if hasLstat {
			info, statErr = srcLstat.Lstat(path)
		} else {
			info, statErr = d.Info()
		}
		if statErr != nil {
			return statErr
		}

		dstPath := pathpkg.Join(root, path)
		return opt.copyEntry(dst, src, path, dstPath, info, nil)
	})
}

// CopyPath copies only the specified paths from src filesystem to dst filesystem.
// Paths must be
func (opt CopyFsOption[Fsys, File]) CopyPath(dst Fsys, src fs.FS, root string, paths ...string) error {
	type pathInfo struct {
		path string
		info fs.FileInfo
	}
	pathInfos := make([]pathInfo, 0, len(paths))
	dirsToCreate := make(map[string]struct{})

	stat := func(path string) (fs.FileInfo, error) {
		return fs.Stat(src, path)
	}
	if srcLstat, ok := src.(interface {
		Lstat(name string) (fs.FileInfo, error)
	}); ok {
		stat = func(path string) (fs.FileInfo, error) {
			return srcLstat.Lstat(path)
		}
	}

	for _, path := range paths {
		path = filepath.Clean(path)
		info, err := stat(filepath.ToSlash(path))
		if err != nil {
			return err
		}
		pathInfos = append(pathInfos, pathInfo{path: path, info: info})
		if info.Mode().IsRegular() {
			dstPath := filepath.Join(root, filepath.FromSlash(path))
			parentDir := filepath.Dir(dstPath)
			if parentDir != "." {
				for dirPath := range pathspkg.PathFromHead(parentDir) {
					dirsToCreate[dirPath] = struct{}{}
				}
			}
		}
	}

	if len(dirsToCreate) > 0 {
		dirs := make([]string, 0, len(dirsToCreate))
		for dir := range dirsToCreate {
			dirs = append(dirs, dir)
		}
		slices.Sort(dirs)

		// Create directories
		for _, dir := range dirs {
			err := dst.Mkdir(dir, fs.ModePerm)
			if err != nil && !errors.Is(err, fs.ErrExist) {
				return err
			}
			// Extract the relative part by removing root prefix
			relDir, err := filepath.Rel(root, dir)
			if err != nil || relDir == "." {
				continue // Skip if we can't get relative path or if it's the root itself
			}
			srcInfo, err := fs.Stat(src, filepath.ToSlash(relDir))
			if err != nil {
				return err
			}
			err = dst.Chmod(dir, opt.maskPerm(srcInfo.Mode()))
			if err != nil {
				return err
			}
		}
	}

	// Second pass: copy all files
	for _, pi := range pathInfos {
		dstPath := filepath.Join(root, filepath.FromSlash(pi.path))
		err := opt.copyEntry(dst, src, pi.path, dstPath, pi.info, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

// copyEntry performs the actual copy operation for a single entry
func (opt CopyFsOption[Fsys, File]) copyEntry(dst Fsys, src fs.FS, srcPath, dstPath string, info fs.FileInfo, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}

	// dstPath is already provided, use it directly
	dstPath = filepath.FromSlash(dstPath)

	// Preserve permissions from source, masked by ChmodMask
	perm := opt.maskPerm(info.Mode())

	var err error
	switch {
	case info.IsDir():
		// Create directory with fs.ModePerm then set proper permissions
		err = dst.Mkdir(dstPath, fs.ModePerm)
		if err != nil && !errors.Is(err, fs.ErrExist) {
			return err
		}

		err = dst.Chmod(dstPath, perm)
		if err != nil {
			return err
		}

	case info.Mode().IsRegular():
		// Copy regular file

		// Open source file
		srcFile, err := src.Open(srcPath)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		// Open destination file with O_TRUNC and O_CREATE
		dstFile, err := dst.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
		if err != nil {
			return err
		}
		defer dstFile.Close()

		// Copy content using io.CopyBuffer
		bufP := bufpool.GetBytes()
		defer bufpool.PutBytes(bufP)

		buf := *bufP
		_, err = io.CopyBuffer(dstFile, srcFile, buf)
		if err != nil {
			return err
		}

	case info.Mode()&fs.ModeSymlink != 0:
		// Handle symlink if src supports ReadLink and dst supports Symlink
		if srcReadLink, hasReadLink := any(src).(ReadLinkFs); hasReadLink {
			if symlinkFs, hasSymlink := any(dst).(SymlinkFs); hasSymlink {
				target, err := srcReadLink.ReadLink(srcPath)
				if err != nil {
					return err
				}
				err = symlinkFs.Symlink(filepath.FromSlash(target), dstPath)
				if err != nil {
					return err
				}
			}
			// If dst doesn't support symlinks, ignore the file
		}
		// If src doesn't support ReadLink, ignore the file

	default:
		// Skip other file types (devices, pipes, etc.)
	}

	return nil
}
