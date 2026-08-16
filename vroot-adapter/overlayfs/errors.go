package overlayfs

// fakeSystemErr spells a system error the running platform has no number for,
// wrapping one it does have so errors.Is still matches something callers can
// test. Mirrors synthfs, which needs the same stand-ins.
type fakeSystemErr struct {
	err error
	msg string
}

func (e *fakeSystemErr) Error() string { return e.msg }

func (e *fakeSystemErr) Unwrap() error { return e.err }
