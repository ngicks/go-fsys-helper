package fsutil

// LockLevel is the strength of an advisory file lock, mirroring what both
// POSIX (flock LOCK_SH/LOCK_EX, fcntl F_RDLCK/F_WRLCK) and Windows
// (LockFileEx with/without LOCKFILE_EXCLUSIVE_LOCK) natively provide.
type LockLevel int

const (
	// LockShared allows other shared holders, excludes exclusive holders.
	LockShared LockLevel = 1 + iota
	// LockExclusive excludes every other holder.
	LockExclusive
)
