//go:build !plan9

package errdef

import "syscall"

var (
	ELOOP     = syscall.ELOOP
	EBADF     = syscall.EBADF
	ENOTEMPTY = syscall.ENOTEMPTY
	EROFS     = syscall.EROFS
)
