package fsutil

import "errors"

var (
	ErrBadInput        = errors.New("bad input")
	ErrBadName         = errors.New("bad name")
	ErrBadPattern      = errors.New("bad pattern")
	ErrMaxRetry        = errors.New("max retry")
	ErrHashSumMismatch = errors.New("hash sum mismatch")
)

func IsPackageErr(err error) bool {
	for _, e := range []error{
		ErrBadInput,
		ErrBadName,
		ErrBadPattern,
		ErrMaxRetry,
		ErrHashSumMismatch,
	} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
