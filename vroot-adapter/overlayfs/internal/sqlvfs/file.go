package sqlvfs

import (
	"errors"
	"io"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/vfs"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// sectorSize mirrors the os VFS's _DEFAULT_SECTOR_SIZE. A backend that writes
// in larger units cannot be detected through vroot, and claiming a smaller one
// would let sqlite journal less than a torn write can damage.
const sectorSize = 4096

var (
	_ vfs.File          = (*file)(nil)
	_ vfs.FileLockState = (*file)(nil)
)

// file adapts a [vroot.File] to [vfs.File].
//
// lock is this handle's sqlite lock level; locker, when the backend provides
// one, carries it down to a whole-file advisory lock. The two-level
// [vroot.Locker] cannot express sqlite's five levels, so RESERVED and above all
// map onto the exclusive lock: that blocks other readers earlier than sqlite
// would, which is harmless for a store that runs locking_mode=EXCLUSIVE.
//
// Contention surfaces in whatever shape the platform gives it; none of it is
// translated here. Where the lock is advisory a contended lock waits rather than
// reporting sqlite3.BUSY, because [vroot.Locker] has no try or timeout form. On
// windows LockFileEx ranges are mandatory and [vroot.Locker] can only take the
// whole file, so a foreign handle is refused by its very first read — sqlite
// reads the database header before it takes any lock — which reaches the caller
// as a disk I/O error instead of a wait. Sqlite's own windows VFS keeps reads
// working by locking only the byte range it reserves for locking, which a
// whole-file lock cannot express; being refused is exclusion too, and stricter
// than waiting. Either way contention means two overlays share one top, which
// the design forbids anyway.
type file struct {
	fsys   vroot.Fs[vroot.File]
	inner  vroot.File
	locker vroot.Locker
	name   string
	flags  vfs.OpenFlag
	lock   vfs.LockLevel
}

func newFile(fsys vroot.Fs[vroot.File], name string, inner vroot.File, flags vfs.OpenFlag) *file {
	locker, _ := inner.(vroot.Locker)
	return &file{
		fsys:   fsys,
		inner:  inner,
		locker: locker,
		name:   name,
		flags:  flags,
	}
}

// Close implements [io.Closer].
func (f *file) Close() error {
	err := f.Unlock(vfs.LOCK_NONE)
	err = errors.Join(err, f.inner.Close())
	if f.flags&vfs.OPEN_DELETEONCLOSE != 0 {
		err = errors.Join(err, f.fsys.Remove(f.name))
	}
	return err
}

// ReadAt implements [io.ReaderAt].
func (f *file) ReadAt(p []byte, off int64) (int, error) {
	n, err := f.inner.ReadAt(p, off)
	if err != nil && errors.Is(err, io.EOF) {
		// sqlite's bridge compares against io.EOF by identity to turn a short read
		// into SQLITE_IOERR_SHORT_READ, which every open of a freshly created
		// database depends on; a wrapped EOF would read as a hard I/O error.
		return n, io.EOF
	}
	return n, err
}

// WriteAt implements [io.WriterAt].
func (f *file) WriteAt(p []byte, off int64) (int, error) {
	return f.inner.WriteAt(p, off)
}

// Truncate implements [vfs.File].
func (f *file) Truncate(size int64) error {
	return f.inner.Truncate(size)
}

// Sync implements [vfs.File].
func (f *file) Sync(flags vfs.SyncFlag) error {
	// SYNC_DATAONLY is served as a full sync: vroot has no fdatasync form.
	return f.inner.Sync()
}

// Size implements [vfs.File].
func (f *file) Size() (int64, error) {
	s, err := f.inner.Stat()
	if err != nil {
		return 0, err
	}
	return s.Size(), nil
}

// Lock implements [vfs.File].
func (f *file) Lock(lock vfs.LockLevel) error {
	if f.lock >= lock {
		return nil
	}
	if lock >= vfs.LOCK_RESERVED && f.flags&vfs.OPEN_READONLY != 0 {
		return sqlite3.IOERR_LOCK
	}
	if f.locker != nil {
		level := vroot.LockExclusive
		if lock == vfs.LOCK_SHARED {
			level = vroot.LockShared
		}
		if err := f.locker.Lock(level); err != nil {
			return err
		}
	}
	f.lock = lock
	return nil
}

// Unlock implements [vfs.File].
func (f *file) Unlock(lock vfs.LockLevel) error {
	if f.lock <= lock {
		return nil
	}
	var err error
	if f.locker != nil {
		if lock == vfs.LOCK_SHARED {
			err = f.locker.Lock(vroot.LockShared)
		} else {
			err = f.locker.Unlock()
		}
	}
	// The level is lowered even on error, as the os VFS does: sqlite asserts on
	// the state it last asked for, not on what the OS granted.
	f.lock = lock
	return err
}

// CheckReservedLock implements [vfs.File].
func (f *file) CheckReservedLock() (bool, error) {
	// A whole-file advisory lock cannot be probed for a foreign RESERVED holder
	// without blocking on it, so this answers from the level this handle holds.
	// The store is the database's only writer, which makes that complete.
	return f.lock >= vfs.LOCK_RESERVED, nil
}

// LockState implements [vfs.FileLockState].
func (f *file) LockState() vfs.LockLevel {
	return f.lock
}

// SectorSize implements [vfs.File].
func (f *file) SectorSize() int {
	return sectorSize
}

// DeviceCharacteristics implements [vfs.File].
func (f *file) DeviceCharacteristics() vfs.DeviceCharacteristic {
	// No guarantee holds for every backend behind vroot, so none is claimed;
	// sqlite then journals as defensively as it can.
	return 0
}
