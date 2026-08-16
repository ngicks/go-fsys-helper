package vroot

import "github.com/ngicks/go-fsys-helper/fsutil"

// LockLevel is the strength of an advisory file lock.
// See [fsutil.LockLevel] for the platform primitives it mirrors.
type LockLevel = fsutil.LockLevel

const (
	// LockShared allows other shared holders, excludes exclusive holders.
	LockShared = fsutil.LockShared
	// LockExclusive excludes every other holder.
	LockExclusive = fsutil.LockExclusive
)

// Locker is an optional extension interface a [File] may implement,
// modeled on go-billy's Lock/Unlock. Assert it with a type switch:
//
//	if l, ok := f.(vroot.Locker); ok { … }
//
// Lock acquires a whole-file advisory lock at the given level: it excludes
// the other users that take the lock too, and does not guarantee protection
// against one that touches the file without taking it (though a platform's
// locks may happen to exclude more, e.g. mandatory locks on Windows). Calling Lock again
// with a different level converts the held lock; conversion is not atomic on
// every platform (it may momentarily drop to unlocked, e.g. on Windows).
// Unlock releases the lock entirely.
//
// WARNING: acquiring the lock may switch the underlying file into
// non-blocking mode as a side effect on some platforms/implementations;
// callers should tolerate that.
type Locker interface {
	Lock(level LockLevel) error
	Unlock() error
}
