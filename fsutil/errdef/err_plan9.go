package errdef

import "io/fs"

type errTy struct {
	Base    error
	Message string
}

func newErr(base error, msg string) error {
	return &errTy{
		Base:    base,
		Message: msg,
	}
}

func (e *errTy) Error() string {
	return e.Message
}

func (e *errTy) Unwrap() error {
	return e.Base
}

// Error variants are just alias for syscall errors.
// Fro plan9, these are defined as error wrapping fs error, e.g. [fs.ErrInvalid], [fs.ErrPermission].
var (
	ELOOP     = newErr(fs.ErrInvalid, "too many levels of symbolic links")
	EBADF     = newErr(fs.ErrInvalid, "bad file descriptor")
	ENOTEMPTY = newErr(fs.ErrInvalid, "directory not empty")
	EROFS     = newErr(fs.ErrPermission, "read-only file system")
)
