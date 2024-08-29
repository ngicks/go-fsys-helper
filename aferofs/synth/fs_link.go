package synth

import (
	"fmt"
	"io"
	"io/fs"
	fspkg "io/fs"
	"iter"
	"os"
	pathpkg "path"
	"strings"
	"syscall"

	"github.com/ngicks/go-fsys-helper/aferofs/internal/bufpool"
)

// AddFile adds a FileView to given path.
// If nonexistent, the path prefix is made as directories with permission of 0o777 before umask.
// If the path prefix contains a file, it returns syscall.ENOTDIR.
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

// MapFsIter adds src contents into fsys.
// mapper generates src path as former of pair and destination path as latter of pair value.
// Former values are used with [NewFsFileView] to refer file in src.
// [Fs.AddFile] is used with former value of pairs to add file views into fsys.
func (fsys *Fs) MapFsIter(src fs.FS, mapper iter.Seq2[string, string]) error {
	for k, v := range mapper {
		view, err := NewFsFileView(src, k)
		if err != nil {
			return fmt.Errorf("referring %q in source fs: %w", k, err)
		}
		err = fsys.AddFile(v, view)
		if err != nil {
			return err
		}
	}
	return nil
}

// Copy walks the file tree rooted under srcRoot of source fs,
// adds file by [Fs.AddFile]
func (fsys *Fs) Copy(fs fs.FS, srcRoot, dstRoot string) error {
	if err := validatePath(srcRoot); err != nil {
		return fmt.Errorf("srcRoot: %w", err)
	}
	if err := validatePath(dstRoot); err != nil {
		return fmt.Errorf("dstRoot: %w", err)
	}
	return fspkg.WalkDir(fs, srcRoot, func(path string, d fspkg.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("WalkDir: %w", err)
		}
		if path == srcRoot || d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		view, err := NewFsFileView(fs, path)
		if err != nil {
			return fmt.Errorf("referring %q in source fs: %w", path, err)
		}
		relPath := path
		if srcRoot != "." {
			relPath, _ = strings.CutPrefix(relPath, srcRoot+"/")
		}
		return fsys.AddFile(pathpkg.Join(dstRoot, relPath), view)
	})
}
