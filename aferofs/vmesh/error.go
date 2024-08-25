package vmesh

import "errors"

var (
	// ErrClosedWithError indicates close of FileData returned an error
	// but is removed from *Fs.
	ErrClosedWithError = errors.New("closed with error")
)
