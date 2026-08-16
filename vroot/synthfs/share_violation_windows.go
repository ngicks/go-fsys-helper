package synthfs

import "syscall"

// errSharingViolation is the actual Windows ERROR_SHARING_VIOLATION; chosen
// so behavior mirrors what *os.File on Windows reports when a file is open
// elsewhere.
//
// https://learn.microsoft.com/en-us/windows/win32/debug/system-error-codes--0-499-#ERROR_SHARING_VIOLATION
var errSharingViolation = syscall.Errno(0x20)
