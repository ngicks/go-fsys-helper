package testhelper

import (
	"fmt"
	"testing"
)

// fakeT is a Test[*fakeT] used to assert that helpers signal failure via Fatalf
// without aborting the parent *testing.T.
type fakeT struct {
	*testing.T
	failed bool
	msg    string
}

func newFakeT(t *testing.T) *fakeT {
	return &fakeT{T: t}
}

func (f *fakeT) Fatalf(format string, args ...any) {
	f.failed = true
	f.msg = fmt.Sprintf(format, args...)
}

func (f *fakeT) Run(name string, fn func(t *fakeT)) bool {
	return f.T.Run(name, func(t *testing.T) {
		fn(&fakeT{T: t})
	})
}

var _ Test[*fakeT] = (*fakeT)(nil)
