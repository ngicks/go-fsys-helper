//go:build unix || (js && wasm) || wasip1

package vroot

import (
	"io/fs"
	"syscall"
)

type fileIdent struct {
	dev   uint64
	inode uint64
}

func fileIdentFromSys(_ Fs, _, _ string, stat fs.FileInfo) (fileIdent, bool) {
	s, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		return fileIdent{}, false
	}
	// on darwin it is int32. so don't remove this conversion.
	return fileIdent{uint64(s.Dev), uint64(s.Ino)}, true
}
