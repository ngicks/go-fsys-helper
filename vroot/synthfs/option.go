package synthfs

import (
	"io/fs"

	"github.com/ngicks/go-fsys-helper/vroot/clock"
)

// Option configures a [Root]. The zero value is valid and behaves identically
// to passing a nil *Option to [NewRoot].
type Option struct {
	// Clock supplies wall-clock timestamps stamped onto newly created or
	// modified nodes. nil → [clock.RealWallClock].
	Clock clock.WallClock

	// Allocator allocates a writable [FileView] for [Root.Create] and
	// [Root.OpenFile] with O_CREATE. nil → [NewMemAllocator] using Clock.
	Allocator FileViewAllocator

	// Umask is the file mode mask applied to permissions passed to Create,
	// Mkdir, MkdirAll, and AddFS scaffolding. It is NOT applied to views bound
	// through AddFile / AddFS, whose source mode is kept verbatim.
	// nil → 0o022. Set to a pointer to 0 to disable masking.
	Umask *fs.FileMode

	// MaskChmodMode narrows or widens the mode passed to Chmod before it lands
	// on a node, mirroring per-OS chmod behavior (see [fsutil.MaskChmodMode]).
	// nil → mask to [fs.ModePerm].
	MaskChmodMode func(fs.FileMode) fs.FileMode

	// DisableOpenFileRemoval, when true, makes Remove return a sharing-violation
	// error if the target still has open handles. This emulates Windows
	// ERROR_SHARING_VIOLATION; POSIX builds use a stand-in error that wraps
	// [syscall.EINVAL] so callers can still distinguish it.
	DisableOpenFileRemoval bool
}

// resolvedOption is Option with all defaults applied. All synthfs internals
// read this rather than the user-facing Option.
type resolvedOption struct {
	clock                  clock.WallClock
	allocator              FileViewAllocator
	umask                  fs.FileMode
	maskChmodMode          func(fs.FileMode) fs.FileMode
	disableOpenFileRemoval bool
}

func (o *Option) resolve() resolvedOption {
	r := resolvedOption{
		umask:         0o022,
		maskChmodMode: func(m fs.FileMode) fs.FileMode { return m & fs.ModePerm },
	}
	if o == nil {
		r.clock = clock.RealWallClock()
		r.allocator = NewMemAllocator(r.clock)
		return r
	}
	r.clock = o.Clock
	if r.clock == nil {
		r.clock = clock.RealWallClock()
	}
	r.allocator = o.Allocator
	if r.allocator == nil {
		r.allocator = NewMemAllocator(r.clock)
	}
	if o.Umask != nil {
		r.umask = *o.Umask
	}
	if o.MaskChmodMode != nil {
		r.maskChmodMode = o.MaskChmodMode
	}
	r.disableOpenFileRemoval = o.DisableOpenFileRemoval
	return r
}
