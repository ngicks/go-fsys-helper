package fsutil

import (
	"io/fs"
	"os"
)

// WrapPathErr wraps error into [*fs.PathError].
//
// If err is nil, WrapPathErr also returns nil.
//
// If err is already a PathError, each field of PathError is overwritten
// by non zero op and/or path.
func WrapPathErr(op, path string, err error) error {
	if err == nil {
		return nil
	}
	pathErr, ok := err.(*fs.PathError)
	if ok {
		if op != "" {
			pathErr.Op = op
		}
		if path != "" {
			pathErr.Path = path
		}
		return err
	}
	return &fs.PathError{Op: op, Path: path, Err: err}
}

// WrapLinkErr wraps error into [*os.LinkError].
//
// If err is nil, WrapLinkErr also returns nil.
//
// If err is already a LinkError, each field of LinkError is overwritten
// by non zero op, old and/or new.
func WrapLinkErr(op, old, new string, err error) error {
	if err == nil {
		return nil
	}
	linkErr, ok := err.(*os.LinkError)
	if ok {
		if op != "" {
			linkErr.Op = op
		}
		if old != "" {
			linkErr.Old = old
		}
		if new != "" {
			linkErr.New = new
		}
		return err
	}
	return &os.LinkError{Op: op, Old: old, New: new, Err: err}
}
