package errdef

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"
)

func Badf(op string, path string) error {
	return &fs.PathError{Op: op, Path: path, Err: syscall.EBADF}
}

func Inval(op string, path string) error {
	return &fs.PathError{Op: op, Path: path, Err: syscall.EINVAL}
}

// read

func ReadBadf(path string) error {
	return &fs.PathError{Op: "read", Path: path, Err: syscall.EBADF}
}

func ReadIsDir(path string) error {
	return &fs.PathError{Op: "read", Path: path, Err: syscall.EISDIR}
}

// readat

func ReadAtBadf(path string) error {
	return &fs.PathError{Op: "readat", Path: path, Err: syscall.EBADF}
}

func ReadAtIsDir(path string) error {
	return &fs.PathError{Op: "readat", Path: path, Err: syscall.EISDIR}
}

// readdir

func ReaddirNotADir(path string) error {
	return &fs.PathError{Op: "readdir", Path: path, Err: syscall.ENOTDIR}
}

// seek

func SeekInval(path string, str string) error {
	return &fs.PathError{Op: "seek", Path: path, Err: fmt.Errorf("%w: "+str, syscall.EINVAL)}
}

// truncate

func TruncateBadf(path string) error {
	return Badf("truncate", path)
}

func TruncateInvalid(path string) error {
	return Inval("truncate", path)
}

// write

func WriteBadf(path string) error {
	return Badf("write", path)
}

// writeat

func WriteAtBadf(path string) error {
	return Badf("writeat", path)
}

var (
	errWriteAtInAppendMode = errors.New("invalid use of WriteAt on file opened with O_APPEND")
)

func WriteAtInAppendMode(path string) error {
	return &fs.PathError{Op: "writeat", Path: path, Err: errWriteAtInAppendMode}
}
