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

	var lastPart string
	if skipLastElement {
		// Use strings.LastIndex to find the last separator and extract the last part
		if idx := strings.LastIndex(name, string(filepath.Separator)); idx >= 0 {
			lastPart = name[idx+1:]
			name = name[:idx]
		} else {
			// No separator found, the entire name is the last part
			return name, nil
		}
	}

	var pathBuilder strings.Builder
	i := 0
	off := 0
	for part := range strings.SplitSeq(name, string(filepath.Separator)) {
		if i > 0 {
			pathBuilder.WriteByte(filepath.Separator)
		}

		off += len(part)
		i++

		pathBuilder.WriteString(part)
		currentPath := pathBuilder.String()

		info, err := fsys.Lstat(currentPath)
		if err != nil {
			return currentPath + name[off+i-1:], err
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

		// Reset the builder and start from the resolved path
		pathBuilder.Reset()
		pathBuilder.WriteString(resolved)
	}

	if lastPart != "" {
		if pathBuilder.Len() > 0 {
			pathBuilder.WriteByte(filepath.Separator)
		}
		pathBuilder.WriteString(lastPart)
	}

	return filepath.ToSlash(pathBuilder.String()), nil
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
