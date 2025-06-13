package vroot

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ngicks/go-fsys-helper/vroot/internal/wrapper"
)

type readLink interface {
	ReadLink(name string) (string, error)
	Lstat(name string) (fs.FileInfo, error)
}

func ResolvePath(fsys readLink, name string, skipLastElement bool) (string, error) {
	name = filepath.Clean(name)

	if name == "." {
		return ".", nil
	}

	if !filepath.IsLocal(name) {
		return "", ErrPathEscapes
	}

	// TODO: use strings.SplitSeq, strings.LastIndex and strings.Builder to
	// minimize allocation.
	parts := strings.Split(name, string(filepath.Separator))
	currentPath := ""

	var lastPart string
	if skipLastElement { // for readlink, lstat, lchown
		lastPart = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	}

	for i, part := range parts {
		if currentPath == "" {
			currentPath = part
		} else {
			currentPath = currentPath + string(filepath.Separator) + part
		}

		info, err := fsys.Lstat(currentPath)
		if err != nil {
			if len(part) > i+1 {
				currentPath += string(filepath.Separator) + filepath.Join(parts[i+1:]...)
			}
			return filepath.Join(currentPath), err
		}

		if info.Mode()&fs.ModeSymlink == 0 {
			continue
		}

		resolved, err := resolveSymlink(fsys, currentPath)
		if err != nil {
			return "", err
		}

		if resolved == "" || !filepath.IsLocal(resolved) {
			// Target is absolute or has "..".
			// *os.Root rejects this anyway, since it cannot tell final result is within root.
			// *os.Root depends on "at" variants of syscalls(e.g. openat.)
			// The root directory may be moved after open,
			// but you don't have robust way to convert an fd back to a path on the filesystem,
			// maybe even multiple paths are linked to a file.
			return "", ErrPathEscapes
		}

		currentPath = resolved
	}

	if lastPart != "" {
		if currentPath != "" {
			currentPath += string(filepath.Separator)
		}
		currentPath += lastPart
	}

	return filepath.ToSlash(currentPath), nil
}

// resolveSymlink resolves a symlink until target is other than symlink.
func resolveSymlink(fsys readLink, linkRealPath string) (string, error) {
	if linkRealPath == "" || linkRealPath == "." {
		return "", nil
	}
	resolved := filepath.Clean(linkRealPath)
	prev := resolved
	prevPrev := ""
	for {
		target, err := fsys.ReadLink(resolved)
		if err != nil {
			return "", err
		}

		target = filepath.Clean(target)

		if filepath.IsAbs(target) {
			// can't tell whether this target is non-symlnk or not,
			// just return ""
			return "", nil
		}

		resolved = filepath.Join(filepath.Dir(resolved), target)

		if !filepath.IsLocal(resolved) {
			// same as absolute path,
			// return just ""
			return "", nil
		}

		if resolved == prevPrev {
			// symlink targeting each other
			return "", wrapper.PathErr("stat", linkRealPath, syscall.ELOOP)
		}

		info, err := fsys.Lstat(resolved)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return resolved, nil
		}

		prevPrev = prev
		prev = resolved
	}
}
