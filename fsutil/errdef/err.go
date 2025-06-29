//go:build !plan9

package errdef

import (
	_ "io/fs"
	"syscall"
)

// Error variants are just alias for syscall errors.
// Fro plan9, these are defined as error wrapping fs error, e.g. [fs.ErrInvalid], [fs.ErrPermission].
var (
	ELOOP     = syscall.ELOOP
	EBADF     = syscall.EBADF
	ENOTEMPTY = syscall.ENOTEMPTY
	EROFS     = syscall.EROFS
)
