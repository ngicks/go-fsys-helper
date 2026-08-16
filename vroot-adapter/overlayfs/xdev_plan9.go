package overlayfs

import "syscall"

// errCrossDevice is what [Fs.renameDirCheck] refuses a directory the layers below
// contribute to with, following the reference. Plan 9 numbers no EXDEV, so
// [syscall.EINVAL] carries the message the other platforms spell with one, the
// way fsutil/errdef stands in for the error numbers plan 9 lacks.
var errCrossDevice error = &fakeSystemErr{
	err: syscall.EINVAL,
	msg: "invalid cross-device link",
}
