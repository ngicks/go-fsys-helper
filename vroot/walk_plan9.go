package vroot

import (
	"io/fs"
	"syscall"
)

type fileIdent struct {
	// system-modified data
	Type uint16      // server type
	Dev  uint32      // server subtype
	Qid  syscall.Qid // unique id from server
}

func fileIdentFromSys(_ Fs, _, _ string, stat fs.FileInfo) (fileIdent, bool) {
	s, ok := stat.Sys().(*syscall.Dir)
	if !ok {
		return fileIdent{}, false
	}
	// Oh is it correct?
	return fileIdent{s.Type, s.Dev, s.Qid}, true
}
