package synth

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"syscall"

	"github.com/ngicks/go-fsys-helper/aferofs/internal/bufpool"
)

// AddFile adds a FileData to given path.
// If nonexistent, the path prefix is made as directories with permission of 0o777 before umask.
// If the path prefix contains a file, syscall.ENOTDIR.
// If basename of path exists before AddFile, it will be removed.
func (f *Fs) AddFile(path string, fileData FileView) error {
	err := validatePath(path)
	if err != nil {
		return wrapErr("AddFile", path, err)
	}
	_, err = f.addFile(path, fileData)
	return wrapErr("AddFile", path, err)
}

func (f *Fs) addFile(path string, fileData FileView) (*dirent, error) {
	dirent, err := newFileDirent(fileData, path)
	if err != nil {
		return nil, err
	}

	dir, base := pathpkg.Split(path)
	if base == "" {
		return nil, wrapErr("AddFile", path, fmt.Errorf("%w: root dir", fs.ErrInvalid))
	}
	dir = pathpkg.Clean(dir)
	err = f.MkdirAll(dir, fs.ModePerm)
	if err != nil {
		return nil, err
	}
	parent, err := f.find(dir)
	if err != nil {
		return nil, err
	}
	if err := parent.IsWritableDir(); err != nil {
		return nil, err
	}

	ent, ok := parent.lookup(base)
	if ok {
		ent.notifyClose()
	}

	parent.addDirent(dirent)
	return dirent, nil
}

// Reallocate allocates a new file using allocator,
// copies the content of path into the new FileData,
// then store it in the fsys.
func (fsys *Fs) Reallocate(path string, allocator FileViewAllocator) error {
	oldDirent, err := fsys.find(path)
	if err != nil {
		return wrapErr("Reallocate", path, err)
	}

	if !oldDirent.IsFile() {
		return wrapErr("Reallocate", path, syscall.EBADF)
	}

	oldFile, err := oldDirent.file.Open(os.O_RDONLY)
	if err != nil {
		return wrapErr("Reallocate", path, err)
	}
	defer oldFile.Close()

	newFD := allocator.Allocate(path, fs.ModePerm)
	newFile, err := newFD.Open(os.O_CREATE | os.O_RDWR)
	if err != nil {
		return err
	}
	defer newFile.Close()

	bytesBuf := bufpool.GetBytes()
	defer bufpool.PutBytes(bytesBuf)

	_, err = io.CopyBuffer(newFile, oldFile, *bytesBuf)
	if err != nil {
		return err
	}

	dirent, err := fsys.addFile(path, newFD)
	if err != nil {
		return wrapErr("AddFile", path, err)
	}

	dirent.copyMeta(oldDirent)

	err = oldDirent.notifyClose()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrClosedWithError, err)
	}

	return nil
}
