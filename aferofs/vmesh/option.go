package vmesh

import "github.com/ngicks/go-fsys-helper/aferofs/clock"

type FsOption interface {
	apply(*Fs)
}

type fsOptionClock [1]clock.WallClock

func (o fsOptionClock) apply(fsys *Fs) {
	fsys.clock = o[0]
}

func WithWallClock(clock clock.WallClock) FsOption {
	return fsOptionClock{clock}
}
