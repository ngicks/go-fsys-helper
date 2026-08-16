//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package osfs

import (
	"syscall"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// The build tag lists the platforms where syscall.Flock exists: aix, hurd and
// solaris (non-illumos) have no Flock in std syscall, and js/wasm, wasip1 and
// plan9 have no advisory file locks at all, so [File] deliberately lacks
// Lock/Unlock there instead of returning a "not supported" error.
var _ vroot.Locker = (*File)(nil)

// Lock implements [vroot.Locker] through flock(2).
//
// flock locks the open file description, not the descriptor: two handles for the
// same path contend with each other even within this process, while descriptors
// duplicated from one handle share a single lock.
func (f *File) Lock(level vroot.LockLevel) error {
	var how int
	switch level {
	case vroot.LockShared:
		how = syscall.LOCK_SH
	case vroot.LockExclusive:
		how = syscall.LOCK_EX
	default:
		return fsutil.WrapPathErr("lock", f.Name(), syscall.EINVAL)
	}
	return fsutil.WrapPathErr("lock", f.Name(), f.flock(how))
}

// Unlock implements [vroot.Locker].
func (f *File) Unlock() error {
	return fsutil.WrapPathErr("unlock", f.Name(), f.flock(syscall.LOCK_UN))
}

func (f *File) flock(how int) error {
	for {
		err := syscall.Flock(int(f.Fd()), how)
		// A blocking flock is interruptible; the runtime's own preemption signals
		// can surface as EINTR even though the call is otherwise restartable.
		if err != syscall.EINTR {
			return err
		}
	}
}
