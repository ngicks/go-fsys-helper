package wrapper

import (
	"io/fs"
	"os"
)

func PathErr(op, path string, err error) error {
	if err == nil {
		return nil
	}
	pathErr, ok := err.(*fs.PathError)
	if ok {
		pathErr.Op = op
		pathErr.Path = path
		return err
	}
	return &fs.PathError{Op: op, Path: path, Err: err}
}

func LinkErr(old, new string, err error) error {
	if err == nil {
		return nil
	}
	linkErr, ok := err.(*os.LinkError)
	if ok {
		linkErr.Op = "link"
		linkErr.Old = old
		linkErr.New = new
		return err
	}
	return &os.LinkError{Op: "link", Old: old, New: new, Err: err}
}
