package fsutil

import (
	"io/fs"
	"os"
)

// WrapPathErr wraps error into [*fs.PathError].
//
// If err is nil, WrapPathErr also returns nil.
//
// If err is already a PathError, a copy is returned with each field overwritten
// by non zero op and/or path; the input error value is never mutated.
func WrapPathErr(op, path string, err error) error {
	if err == nil {
		return nil
	}
	pathErr, ok := err.(*fs.PathError)
	if ok {
		// Copy-on-write: never mutate the caller's error value (it may be a
		// shared sentinel, stored, or observed by another goroutine).
		cp := *pathErr
		if op != "" {
			cp.Op = op
		}
		if path != "" {
			cp.Path = path
		}
		return &cp
	}
	return &fs.PathError{Op: op, Path: path, Err: err}
}

// WrapLinkErr wraps error into [*os.LinkError].
//
// If err is nil, WrapLinkErr also returns nil.
//
// If err is already a LinkError, a copy is returned with each field overwritten
// by non zero op, old and/or new; the input error value is never mutated.
func WrapLinkErr(op, old, new string, err error) error {
	if err == nil {
		return nil
	}
	linkErr, ok := err.(*os.LinkError)
	if ok {
		// Copy-on-write: never mutate the caller's error value (it may be a
		// shared sentinel, stored, or observed by another goroutine).
		cp := *linkErr
		if op != "" {
			cp.Op = op
		}
		if old != "" {
			cp.Old = old
		}
		if new != "" {
			cp.New = new
		}
		return &cp
	}
	return &os.LinkError{Op: op, Old: old, New: new, Err: err}
}
