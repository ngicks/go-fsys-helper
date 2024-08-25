package vmesh

import (
	"io/fs"

	"github.com/spf13/afero"
)

// FileDataAllocator allocates new FileData at path.
type FileDataAllocator interface {
	Allocate(path string, perm fs.FileMode) FileData
}

// FileData is a pointer to a file stored in the underlying system.
//
// FileData is currently only assumed to be a regular file.
type FileData interface {
	// Open opens this FileData.
	// Implementations may or may not ignore flag.
	//
	// Open should return a newly created file handle.
	// *Fs may call Open many times and each should be handled individually.
	// Therefore some attributes, e.g. file offset, should be managed separately.
	//
	// flag is same that you can use with os.OpenFile,
	// namely one of os.O_RDONLY, os.O_WRONLY or os.O_RDWR bitwise-or'ed
	// with any or none of os.O_APPEND, os.O_CREATE, os.O_EXCL, os.O_SYNC or os.O_TRUNC.
	Open(flag int) (afero.File, error)
	// Stat is a short hand for Open and then Stat on afero.File.
	Stat() (fs.FileInfo, error)
	// Truncate is a short hand for Open then Truncate on afero.File.
	// Readonly implementations may return a bare syscall.EROFS, or similar errors.
	Truncate(size int64) error
	// Close notifies the underlying file data
	// that this FileData is no longer referred by name.
	//
	// The file opened by calling Open method may still
	// exist and be used.
	Close() error
}
