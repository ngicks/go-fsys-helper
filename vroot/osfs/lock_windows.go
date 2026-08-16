package osfs

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

var _ vroot.Locker = (*File)(nil)

// LockFileEx takes a byte range; the whole file is expressed as offset 0 (the
// zero value of the Overlapped it reads the offset from) and the maximum length.
const lockAllBytes = ^uint32(0)

// Lock implements [vroot.Locker] through LockFileEx.
//
// Windows has no primitive that converts a held range lock in place, so Lock
// releases whatever this handle holds before acquiring the new level; the window
// between the two is the non-atomic conversion [vroot.Locker] warns about.
// Dropping first also keeps repeated Lock calls from stacking shared locks, which
// LockFileEx grants per call and would each need their own UnlockFileEx.
func (f *File) Lock(level vroot.LockLevel) error {
	var flags uint32
	switch level {
	case vroot.LockShared:
		flags = 0
	case vroot.LockExclusive:
		flags = windows.LOCKFILE_EXCLUSIVE_LOCK
	default:
		return fsutil.WrapPathErr("lock", f.Name(), syscall.EINVAL)
	}

	if err := f.unlockFile(); err != nil && !errors.Is(err, windows.ERROR_NOT_LOCKED) {
		return fsutil.WrapPathErr("lock", f.Name(), err)
	}

	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		flags,
		0,
		lockAllBytes,
		lockAllBytes,
		new(windows.Overlapped),
	)
	return fsutil.WrapPathErr("lock", f.Name(), err)
}

// Unlock implements [vroot.Locker].
func (f *File) Unlock() error {
	return fsutil.WrapPathErr("unlock", f.Name(), f.unlockFile())
}

func (f *File) unlockFile() error {
	return windows.UnlockFileEx(
		windows.Handle(f.Fd()),
		0,
		lockAllBytes,
		lockAllBytes,
		new(windows.Overlapped),
	)
}
