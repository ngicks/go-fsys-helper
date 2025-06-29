package synthfs

import (
	"io/fs"
	"os"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// FileViewAllocator allocates new FileView at path.
type FileViewAllocator interface {
	Allocate(path string, perm fs.FileMode) FileView
}

// FileView is a pointer to a file-like data stored in a backing storage.
//
// FileView is currently only assumed to be a regular file.
type FileView interface {
	// Open opens this FileView.
	// Implementations may or may not ignore flag.
	//
	// Open should return a newly created file handle.
	// *Fs may call Open many times and may return results as different files.
	// Therefore some attributes, e.g. file offset, should be managed separately.
	//
	// flag is same that you can use with os.OpenFile,
	// namely one of os.O_RDONLY, os.O_WRONLY or os.O_RDWR bitwise-or'ed
	// with any or none of os.O_APPEND, os.O_CREATE, os.O_EXCL, os.O_SYNC or os.O_TRUNC.
	Open(flag int) (vroot.File, error)
	// Close notifies the backing storage
	// that this FileView is no longer referred by name.
	//
	// The file opened by calling Open method may still
	// exist and be used.
	//
	// The returned error might be ignored.
	Close() error
}

func statFileView(fv FileView) (fs.FileInfo, error) {
	f, err := fv.Open(os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}
