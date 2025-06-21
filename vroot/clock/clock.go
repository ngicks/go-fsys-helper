package clock

import "time"

// WallClock is an interface wrapping basic Now method, which returns wall clock time.
// For real clock that wraps [time.Now], use [RealWallClock].
//
// Currently this packages does not provide a mock implementation.
// You can implement your own or you can use [github.com/jonboulle/clockwork]
type WallClock interface {
	Now() time.Time
}

type realWallClock struct{}

func (c realWallClock) Now() time.Time {
	return time.Now()
}

func RealWallClock() WallClock {
	return realWallClock{}
}
