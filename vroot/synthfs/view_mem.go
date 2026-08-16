package synthfs

import (
	"io/fs"
	"path"

	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/clock"
)

// memAllocator is the default [FileViewAllocator] — it backs each newly
// created file with an in-memory byte slice managed by [memBuf].
type memAllocator struct {
	clock clock.WallClock
}

// NewMemAllocator returns a [FileViewAllocator] that allocates writable
// in-memory views. c supplies modTime updates on writes; nil → [clock.RealWallClock].
func NewMemAllocator(c clock.WallClock) FileViewAllocator {
	if c == nil {
		c = clock.RealWallClock()
	}
	return &memAllocator{clock: c}
}

func (a *memAllocator) Allocate(name string, perm fs.FileMode) (FileView, error) {
	return &memView{
		name: name,
		buf: &memBuf{
			clock:   a.clock,
			mode:    perm & fs.ModePerm,
			modTime: a.clock.Now(),
		},
	}, nil
}

// memView is the [FileView] over an in-memory buffer.
type memView struct {
	name string
	buf  *memBuf
}

func (v *memView) Open(flag int) (vroot.File, error) {
	return &memHandle{buf: v.buf, name: v.name, flag: flag}, nil
}

func (v *memView) Stat() (fs.FileInfo, error) {
	return v.buf.stat(path.Base(v.name)), nil
}

func (v *memView) Close() error {
	return nil
}
