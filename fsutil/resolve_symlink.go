package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ErrPathEscapes is retuned when evaluating the path reuslts to out of root
// (a path starts with ".." or an absolute path.)
var ErrPathEscapes = errors.New("path escapes from parent")

// ResolvePath resolves symbolic links in the given path by traversing each
// path component and following symlinks when encountered.
//
// If skipLastElement is true, the final path component is preserved without
// symlink resolution (useful for operations on the symlink itself e.g. Lstat, Lchown).
//
// Returns ErrPathEscapes for paths that would escape the parent directory (including absolute paths.)
// When a path component doesn't exist, returns an error that satisfies errors.Is(err, fs.ErrNotExist)
// with the path concatenated from the intermediate resolution result and remaining unresolved components.
//
// ResolvePath is still vulnerable to attack using TOCTOU(Time Of Check Time Of Use) race;
// unlike [*os.Root] which leverages openat and lstatat, ResolvePath is just a sequence of Lstat and ReadLink.
func ResolvePath(
	fsys interface {
		ReadLinkFs
		LstatFs
	},
	name string,
	skipLastElement bool,
) (string, error) {
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
		idx := strings.LastIndex(name, string(filepath.Separator))
		if idx < 0 {
			// No separator found, the entire name is the last part
			return name, nil
		}
		lastPart = name[idx+1:]
		name = name[:idx]
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

		resolved, err := ResolveSymlink(fsys, currentPath)
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

// ResolveSymlink resolves a symlink.
//
// linkRealPath must be a real path for a link.
// Otherwise it might return an error that satisfies errors.Ie(err, fs.ErrNotExist),
// or even an incorrect path.
//
// If linkRealPath is a link to another link, ResolvePath resolves that link until it finds
// a file other than a symlink.
//
// In case linkRealPath is a link to other link and that link targets to the file pointed by linkRealPath,
// then ResolveSymlink returns an error that satisfies error.Is(err, syscall.ELOOP).
func ResolveSymlink(
	fsys interface {
		ReadLinkFs
		LstatFs
	},
	linkRealPath string,
) (string, error) {
	if linkRealPath == "" || linkRealPath == "." {
		return "", nil
	}
	resolved := filepath.Clean(linkRealPath)
	for range 40 { // following linux's maxium: https://man7.org/linux/man-pages/man7/path_resolution.7.html
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

		info, err := fsys.Lstat(resolved)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return resolved, nil
		}
	}

	return "", WrapPathErr("stat", linkRealPath, syscall.ELOOP)
}
