package synthfs

import (
	"io/fs"

	"github.com/ngicks/go-fsys-helper/vroot"
)

// FileView is a pointer to a file's bytes in some backing storage
// (in-memory buffer, fs.FS file, byte range of another view, generated
// bytes, …). synthfs uses views to populate regular files without copying
// their contents into the tree.
//
// A view is bound to a regular file; directories and symlinks do not have
// views.
type FileView interface {
	// Open returns a fresh handle. Per-handle state such as the seek offset
	// is owned by the returned [vroot.File], so Open may be called multiple
	// times against the same view. flag is os.OpenFile-style; read-only views
	// should reject write flags with [github.com/ngicks/go-fsys-helper/fsutil/errdef.EROFS].
	Open(flag int) (vroot.File, error)
	// Stat reports the view's current metadata. The synthfs tree treats Size
	// as authoritative (read live every time); Mode and ModTime are sampled
	// once when the view is bound and then owned by the tree node.
	Stat() (fs.FileInfo, error)
	// Close is called when the view is unlinked from the tree (e.g. removed
	// or renamed over). Handles previously returned by Open may outlive the
	// view; Close should not invalidate them.
	Close() error
}

// FileViewAllocator allocates a fresh, writable view for a newly created
// regular file. The default allocator ([NewMemAllocator]) backs each file
// with an in-memory buffer.
type FileViewAllocator interface {
	// Allocate returns a fresh writable view for the file that will be bound
	// at name with the given perm. The allocator owns the view's storage; the
	// tree will call Close on the returned view when it is unlinked.
	Allocate(name string, perm fs.FileMode) (FileView, error)
}
