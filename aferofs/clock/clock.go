package clock

import "time"

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
