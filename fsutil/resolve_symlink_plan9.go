package fsutil

import "syscall"

func resolvePath(
	_ interface {
		ReadLinkFs
		LstatFs
	},
	name string,
	_ bool,
) (string, error) {
	return "", WrapPathErr("lstat", name, syscall.EPLAN9)
}

func resolveSymlink(
	_ interface {
		ReadLinkFs
		LstatFs
	},
	linkRealPath string,
	_ int,
) (resolved string, numSymlink int, err error) {
	return "", -1, WrapPathErr("readlink", linkRealPath, syscall.EPLAN9)
}
