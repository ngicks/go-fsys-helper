package memfs

import (
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/clock"
	"github.com/ngicks/go-fsys-helper/vroot/synthfs"
)

func NewRooted(name string) vroot.Rooted {
	return synthfs.NewRooted(
		name,
		synthfs.NewMemFileAllocator(clock.RealWallClock()),
		synthfs.Option{
			Clock: clock.RealWallClock(),
		},
	)
}

func NewUnrooted(name string) vroot.Unrooted {
	return synthfs.NewUnrooted(
		name,
		synthfs.NewMemFileAllocator(clock.RealWallClock()),
		synthfs.Option{
			Clock: clock.RealWallClock(),
		},
	)
}
