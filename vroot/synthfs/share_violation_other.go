//go:build !windows

package synthfs

import "syscall"

// errSharingViolation is the error returned by Remove when
// [Option.DisableOpenFileRemoval] is true and the target still has open
// handles. On non-Windows builds we wrap [syscall.EINVAL] with a Windows-like
// message so user-facing diagnostics read sensibly, while errors.Is(err,
// syscall.EINVAL) still matches.
var errSharingViolation = &fakeSystemErr{
	err: syscall.EINVAL,
	msg: "The process cannot access the file because it is being used by another process.",
}

type fakeSystemErr struct {
	err error
	msg string
}

func (e *fakeSystemErr) Error() string { return e.msg }
func (e *fakeSystemErr) Unwrap() error { return e.err }
