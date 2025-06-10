//go:build unix

package vroot

import (
	"io/fs"
	"syscall"
)

func fileIdentFromSys(_ Fs, _ string, stat fs.FileInfo) (fileIdent, bool) {
	s, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		return fileIdent{}, false
	}
	return fileIdent{s.Dev, s.Ino}, true
}
