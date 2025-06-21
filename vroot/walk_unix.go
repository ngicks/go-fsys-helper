//go:build unix

package vroot

import (
	"io/fs"
	"syscall"
)

func fileIdentFromSys(_ Fs, _, _ string, stat fs.FileInfo) (fileIdent, bool) {
	s, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		return fileIdent{}, false
	}
	// on darwin it is int32. so don't remove this conversion.
	return fileIdent{uint64(s.Dev), uint64(s.Ino)}, true
}
