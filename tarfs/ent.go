package tarfs

import (
	"errors"
	"io"
	"io/fs"
)

// direntry is static, stateless entry in the [*Fs].
type direntry interface {
	header() *Section
	open(r io.ReaderAt, path string) openDirentry
}

// openDirentry is the stateful file opened through [direntry].
// It has states, i.e. offset for file reading or ReadDir.
type openDirentry interface {
	Name() string
	fs.File
}

func pathErr(op, path string, err error) error {
	if err == nil {
		return nil
	}
	if err == io.EOF {
		return err
	}
	return &fs.PathError{
		Op:   op,
		Path: path,
		Err:  err,
	}
}

func overrideErr(err error, cb func(err *fs.PathError)) {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		cb(pathErr)
	}
}
