package vroot

import (
	"io/fs"
	"syscall"
)

func fileIdentFromSys(fsys Fs, realPath string, _ fs.FileInfo) (fileIdent, bool) {
	f, err := fsys.Open(realPath)
	if err != nil {
		return fileIdent{}, false
	}
	defer f.Close()

	fd := f.Fd()
	if fd == 0xffffffff { // invalid value
		return fileIdent{}, false
	}

	var info syscall.ByHandleFileInformation
	err = syscall.GetFileInformationByHandle(syscall.Handle(fd), &info)
	if err != nil {
		return fileIdent{}, false
	}
	return fileIdent{
		dev:   uint64(info.VolumeSerialNumber),
		inode: (uint64(info.FileIndexHigh) << 32) | uint64(info.FileIndexLow),
	}, true
}
