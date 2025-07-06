package fsutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ngicks/go-fsys-helper/fsutil/errdef"
)

// following linux's maxium: https://man7.org/linux/man-pages/man7/path_resolution.7.html
const maxSymlinkResolutionCount = 40

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
// unlike [*os.Root] which leverages APIs that open paths relative from open file handle (e.g. openat(2) and fstatat(2)),
// ResolvePath is just a sequence of Lstat and ReadLink.
//
// For plan9, if underlying fsys does not have concept of symlink, it would just returns bare syscall.EPLAN9
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

	nameWihoutLastPart := name
	var lastPart string
	if skipLastElement {
		// Use strings.LastIndex to find the last separator and extract the last part
		idx := strings.LastIndex(name, string(filepath.Separator))
		if idx < 0 {
			// No separator found, the entire name is the last part
			return name, nil
		}
		lastPart = name[idx+1:]
		nameWihoutLastPart = name[:idx]
	}

	currentSymlinkResolutionCount := 0

	var pathBuilder strings.Builder
	for i, part := range splitPathSeq(nameWihoutLastPart) {
		if i > 0 {
			pathBuilder.WriteByte(filepath.Separator)
		}

		pathBuilder.WriteString(part.component)
		currentPath := pathBuilder.String()

		info, err := fsys.Lstat(currentPath)
		if err != nil {
			rest := name[min(len(name), part.offsetEnd):]
			if len(rest) > 0 {
				return currentPath + string(filepath.Separator) + rest, err
			}
			return currentPath, err
		}

		if info.Mode()&fs.ModeSymlink == 0 {
			continue
		}

		resolved, numSymlink, err := ResolveSymlink(
			fsys,
			currentPath,
			maxSymlinkResolutionCount-currentSymlinkResolutionCount,
		)
		if err != nil {
			return "", err
		}

		currentSymlinkResolutionCount += numSymlink
		if currentSymlinkResolutionCount >= maxSymlinkResolutionCount {
			return "", WrapPathErr("stat", name, errdef.ELOOP)
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

	return pathBuilder.String(), nil
}

// ResolveSymlink retruns resoluition result with numbers of symlink that has been resolved.
//
// linkRealPath must be a real path for a link.
// Otherwise it might return an error that satisfies errors.Ie(err, fs.ErrNotExist),
// or even an incorrect path.
//
// If linkRealPath is a link to another link, ResolvePath resolves that link until it finds
// a file other than a symlink.
//
// If symlink is resolved more times than maxResolution,
// it would return an error that satisfies errors.Is(err, [errdef.ELOOP])
//
// For plan9, it may return an error satisfying errors.Is(err, syscall.EPLAN9).
func ResolveSymlink(
	fsys interface {
		ReadLinkFs
		LstatFs
	},
	linkRealPath string,
	maxResolution int,
) (resolved string, numSymlink int, err error) {
	resolved = filepath.Clean(linkRealPath)
	var i int
	for i = 0; i < maxResolution; i++ {
		target, err := fsys.ReadLink(resolved)
		if err != nil {
			return "", i, err
		}

		target = filepath.Clean(target)

		if filepath.IsAbs(target) {
			// can't tell whether this target is non-symlnk or not,
			// just return ""
			return "", i + 1, nil
		}

		resolved = filepath.Join(filepath.Dir(resolved), target)

		if !filepath.IsLocal(resolved) {
			// same as absolute path,
			// return just ""
			return "", i + 1, nil
		}

		info, err := fsys.Lstat(resolved)
		if err != nil {
			return "", i, err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return resolved, i + 1, nil
		}
	}

	return "", i, WrapPathErr("stat", linkRealPath, errdef.ELOOP)
}
