//go:build !unix

package vroot

import (
	"io/fs"
)

func inodeFromSys(_ fs.FileInfo) (inode, bool) {
	return inode{}, false
}
