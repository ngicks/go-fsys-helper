//go:build unix

package vroot

import (
	"io/fs"
	"syscall"
)

func inodeFromSys(stat fs.FileInfo) (inode, bool) {
	s, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		return inode{}, false
	}
	return inode{s.Dev, s.Ino}, true
}
