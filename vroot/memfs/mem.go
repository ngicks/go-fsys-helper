package memfs

import (
	"github.com/ngicks/go-fsys-helper/vroot"
	"github.com/ngicks/go-fsys-helper/vroot/clock"
	"github.com/ngicks/go-fsys-helper/vroot/synthfs"
)

func NewRooted() vroot.Rooted {
	return synthfs.NewRooted(
		"mem:///",
		synthfs.NewMemFileAllocator(clock.RealWallClock()),
		synthfs.Option{
			Clock: clock.RealWallClock(),
		},
	)
}

func NewUnrooted() vroot.Unrooted {
	return synthfs.NewUnrooted(
		"mem:///",
		synthfs.NewMemFileAllocator(clock.RealWallClock()),
		synthfs.Option{
			Clock: clock.RealWallClock(),
		},
	)
}
