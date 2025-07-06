package synthfs

import "syscall"

// https://learn.microsoft.com/ja-jp/windows/win32/debug/system-error-codes--0-499-#ERROR_SHARING_VIOLATION
var ERROR_SHARING_VIOLATION = syscall.Errno(0x20)
