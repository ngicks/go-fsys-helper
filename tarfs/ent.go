package tarfs

import (
	"errors"
	"io"
	"io/fs"
)

type direntry interface {
	header() *header
	open(r io.ReaderAt, path string) opnenDirentry
}

type opnenDirentry interface {
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
