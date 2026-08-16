//go:build !windows

package overlayfs

import "syscall"

// errSharingViolation is returned by Remove/RemoveAll when
// [Option.DisableOpenFileRemoval] is set and the target still has handles open
// through the overlay. On non-Windows builds [syscall.EINVAL] is wrapped in a
// Windows-like message so diagnostics read the same everywhere, while
// errors.Is(err, syscall.EINVAL) still matches. Mirrors synthfs.
var errSharingViolation error = &fakeSystemErr{
	err: syscall.EINVAL,
	msg: "The process cannot access the file because it is being used by another process.",
}
