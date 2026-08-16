package overlayfs

import "syscall"

// errSharingViolation is the actual Windows ERROR_SHARING_VIOLATION, returned by
// Remove/RemoveAll when [Option.DisableOpenFileRemoval] is set and the target
// still has handles open through the overlay. Chosen so behavior mirrors what
// *os.File on Windows reports when a file is open elsewhere. Mirrors synthfs.
//
// https://learn.microsoft.com/en-us/windows/win32/debug/system-error-codes--0-499-#ERROR_SHARING_VIOLATION
var errSharingViolation error = syscall.Errno(0x20)
