package fsutil

import (
	"errors"
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
// For plan9, it just returns an error satisfying errors.Is(err, syscall.EPLAN9)
func ResolvePath(
	fsys interface {
		ReadLinkFs
		LstatFs
	},
	name string,
	skipLastElement bool,
) (string, error) {
	return resolvePath(fsys, name, skipLastElement)
}

// ResolveSymlink retruns resoluition result with numbers of symlink resolved.
//
// linkRealPath must be a real path for a link.
// Otherwise it might return an error that satisfies errors.Ie(err, fs.ErrNotExist),
// or even an incorrect path.
//
// If linkRealPath is a link to another link, ResolvePath resolves that link until it finds
// a file other than a symlink.
//
// If symlink is resolved more times than maxResolution,
// it would return an error that satisfies errors.Is(err, syscall.ELOOP)
//
// For plan9, it just returns an error satisfying errors.Is(err, syscall.EPLAN9)
func ResolveSymlink(
	fsys interface {
		ReadLinkFs
		LstatFs
	},
	linkRealPath string,
	maxResolution int,
) (resolved string, numSymlink int, err error) {
	return resolveSymlink(fsys, linkRealPath, maxResolution)
}
