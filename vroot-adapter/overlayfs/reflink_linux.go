package overlayfs

import (
	"golang.org/x/sys/unix"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// reflinkFile clones src's extents into dst with FICLONE and reports whether
// that worked, leaving dst untouched when it did not.
//
// Both files have to be os-backed for the ioctl to have anything to work on;
// [vroot.File.Fd] says so by returning something other than the ^uintptr(0)
// sentinel. A file that lies about that is out of scope, as PLAN §8 records.
//
// Every failure is one answer — false — and none of them is remembered: whether
// the ioctl said the filesystem has no reflinks, that the two files are on
// different ones, or that this pair in particular could not be cloned, the next
// pair is asked again. The reference latches a global "no reflink here" flag on
// the first refusal; a vroot overlay can span filesystems within one top, so a
// latch would answer for a pair it never tried (PLAN §8).
func reflinkFile(dst, src vroot.File) bool {
	dstFd, srcFd := dst.Fd(), src.Fd()
	if dstFd == ^uintptr(0) || srcFd == ^uintptr(0) {
		return false
	}
	return unix.IoctlFileClone(int(dstFd), int(srcFd)) == nil
}
