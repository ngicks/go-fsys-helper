package vroot

import (
	"io/fs"
	"syscall"
)

// fileIdent is combination of device number and inode of the file for unix systems.
// VolumeSerialNumber and FileIndex for windows system.
type fileIdent struct {
	VolumeSerialNumber          uint32
	FileIndexHigh, FileIndexLow uint32
}

func fileIdentFromSys(fsys Fs, virtualPath, _ string, _ fs.FileInfo) (fileIdent, bool) {
	f, err := fsys.Open(virtualPath)
	if err != nil {
		return fileIdent{}, false
	}
	defer f.Close()

	fd := f.Fd()
	if fd == ^(uintptr(0)) { // invalid value
		return fileIdent{}, false
	}

	var info syscall.ByHandleFileInformation
	err = syscall.GetFileInformationByHandle(syscall.Handle(fd), &info)
	if err != nil {
		return fileIdent{}, false
	}
	return fileIdent{
		info.VolumeSerialNumber,
		info.FileIndexHigh,
		info.FileIndexLow,
	}, true
}
