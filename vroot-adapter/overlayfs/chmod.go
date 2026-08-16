package overlayfs

import (
	"io/fs"
	"time"

	"github.com/ngicks/go-fsys-helper/fsutil"
	"github.com/ngicks/go-fsys-helper/vroot"
)

// Chmod sets name's mode, following name when it is a symlink. The file is
// copied up first: a mode belongs to the copy the overlay owns, and a lower is
// never written to.
func (f *Fs) Chmod(name string, mode fs.FileMode) error {
	return fsutil.WrapPathErr("chmod", name, f.mutateTop(name, false,
		func(fsys vroot.Fs[vroot.File], path string) error { return fsys.Chmod(path, mode) },
	))
}

// Chown sets name's owner, following name when it is a symlink, on the top's
// copy of it.
//
// A backend with no notion of ownership answers with whatever it spells that
// with; the copy-up still happens, since the overlay cannot tell in advance.
func (f *Fs) Chown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("chown", name, f.mutateTop(name, false,
		func(fsys vroot.Fs[vroot.File], path string) error { return fsys.Chown(path, uid, gid) },
	))
}

// Chtimes sets name's times, following name when it is a symlink, on the top's
// copy of it.
func (f *Fs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return fsutil.WrapPathErr("chtimes", name, f.mutateTop(
		name,
		false,
		func(fsys vroot.Fs[vroot.File], path string) error { return fsys.Chtimes(path, atime, mtime) },
	))
}

// Lchown sets name's owner without following name when it is a symlink.
//
// The copy-up brings the link itself into the top, re-created from its target
// (§4.3), so the ownership lands on the overlay's own link rather than on
// whatever it points at.
func (f *Fs) Lchown(name string, uid int, gid int) error {
	return fsutil.WrapPathErr("lchown", name, f.mutateTop(name, true,
		func(fsys vroot.Fs[vroot.File], path string) error { return fsys.Lchown(path, uid, gid) },
	))
}
