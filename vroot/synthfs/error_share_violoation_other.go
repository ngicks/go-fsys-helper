//go:build !windows

package synthfs

import (
	"syscall"
)

var ERROR_SHARING_VIOLATION = newFakeSystemErr(
	syscall.EINVAL,
	"The process cannot access the file because it is being used by another process.",
)

type fakeSystemErr struct {
	err error
	msg string
}

func newFakeSystemErr(err error, msg string) *fakeSystemErr {
	return &fakeSystemErr{
		err: err,
		msg: msg,
	}
}

func (e *fakeSystemErr) Error() string {
	return e.msg
}

func (e *fakeSystemErr) Unwrap() error {
	return e.err
}
