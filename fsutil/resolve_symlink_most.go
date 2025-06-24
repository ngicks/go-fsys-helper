//go:build !plan9

package fsutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func resolvePath(
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
			return "", WrapPathErr("stat", name, syscall.ELOOP)
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

func resolveSymlink(
	fsys interface {
		ReadLinkFs
		LstatFs
	},
	linkRealPath string,
	maxResolution int,
) (resolved string, numSymlink int, err error) {
	if linkRealPath == "" || linkRealPath == "." {
		return "", 0, nil
	}

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

	return "", i, WrapPathErr("stat", linkRealPath, syscall.ELOOP)
}
