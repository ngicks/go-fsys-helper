package fsutil

import (
	"bytes"
	"cmp"
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

	var lastPart string
	if skipLastElement {
		idx := strings.LastIndex(name, string(filepath.Separator))
		if idx < 0 {
			return name, nil
		}
		lastPart = name[idx+1:]
		name = name[:idx]
	}

	curLinkResolved := 0

	resolved := new(bytes.Buffer)

	i := 0
	offStart := 0
	offEnd := 0
	for offStart < len(name) {
		if i > 0 {
			resolved.WriteByte(filepath.Separator)
		}

		if idx := strings.Index(name[offStart:], string(filepath.Separator)); idx >= 0 {
			offEnd = offStart + idx
		} else {
			offEnd = len(name)
		}

		resolved.WriteString(name[offStart:offEnd])

		info, err := fsys.Lstat(resolved.String())
		if err != nil {
			if offEnd < len(name) {
				resolved.WriteString(name[offEnd:])
			}
			return resolved.String(), err
		}

		if info.Mode()&fs.ModeSymlink == 0 {
			offStart = offEnd + 1
			i++
			continue
		}

		currentResolved, numSymlink, err := ResolveSymlink(
			fsys,
			resolved.String(),
			maxSymlinkResolutionCount-curLinkResolved,
		)
		if err != nil {
			return resolved.String(), err
		}

		curLinkResolved += numSymlink
		if curLinkResolved >= maxSymlinkResolutionCount {
			return cmp.Or(currentResolved, resolved.String()), WrapPathErr("stat", name, errdef.ELOOP)
		}

		if currentResolved == "" || !filepath.IsLocal(currentResolved) {
			// Target is absolute or has "..".
			// *os.Root rejects this anyway, since it cannot tell final result is within root.
			// *os.Root depends on "at" variants of syscalls(e.g. openat.)
			// The root directory may be moved after open,
			// but you don't have robust way to convert an fd back to a path on the filesystem,
			// maybe even multiple paths are linked to a file.
			return cmp.Or(currentResolved, resolved.String()), ErrPathEscapes
		}

		if offEnd >= len(name) {
			name = currentResolved
		} else {
			name = currentResolved + name[offEnd:]
		}
		i = 0
		offStart = 0
		offEnd = 0
		resolved.Reset()
	}

	if lastPart != "" {
		if resolved.Len() > 0 {
			resolved.WriteByte(filepath.Separator)
		}
		resolved.WriteString(lastPart)
	}

	return resolved.String(), nil
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
