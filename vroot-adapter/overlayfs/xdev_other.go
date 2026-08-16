//go:build !plan9

package overlayfs

import "syscall"

// errCrossDevice is what [Fs.renameDirCheck] refuses a directory the layers below
// contribute to with, following the reference.
var errCrossDevice error = syscall.EXDEV
